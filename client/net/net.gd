## Connection to the game server (autoload "Net").
##
## Flow: login() does the HTTP dev login, opens the WebSocket and sends Hello.
## Server messages arrive as signals.
extends Node

signal welcomed(welcome: PB.Welcome)
signal snapshot_received(snapshot: PB.Snapshot)
signal disconnected(reason: String)

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const PING_INTERVAL := 2.0

var rtt_ms := 0.0

var _ws: WebSocketPeer
var _token := ""
var _hello_sent := false
var _was_open := false
var _ping_timer := 0.0
var _last_error := ""


## Logs in and connects. Returns "" on success or an error message.
func login(server: String, player_name: String, model: String) -> String:
	close()
	var http := HTTPRequest.new()
	add_child(http)
	var body := JSON.stringify({"name": player_name, "model": model})
	var err := http.request(server + "/api/v1/dev-login",
			PackedStringArray(["Content-Type: application/json"]), HTTPClient.METHOD_POST, body)
	if err != OK:
		http.queue_free()
		return "Login request failed (%s)" % error_string(err)
	var res: Array = await http.request_completed
	http.queue_free()
	if res[0] != HTTPRequest.RESULT_SUCCESS:
		return "Cannot reach the server at %s" % server
	var text := (res[3] as PackedByteArray).get_string_from_utf8().strip_edges()
	if res[1] != 200:
		return "Login refused: %s" % text
	var data = JSON.parse_string(text)
	if typeof(data) != TYPE_DICTIONARY or not data.has("token"):
		return "Unexpected login response"

	_token = data["token"]
	var ws_url := server.replace("https://", "wss://").replace("http://", "ws://") + str(data["ws_path"])
	_ws = WebSocketPeer.new()
	err = _ws.connect_to_url(ws_url)
	if err != OK:
		_ws = null
		return "WebSocket connection failed (%s)" % error_string(err)
	_hello_sent = false
	_was_open = false
	_last_error = ""
	return ""


func close() -> void:
	if _ws != null:
		_ws.close()
		_ws = null


func is_open() -> bool:
	return _ws != null and _ws.get_ready_state() == WebSocketPeer.STATE_OPEN


func send(msg: PB.ClientMessage) -> void:
	if is_open():
		_ws.send(msg.to_bytes(), WebSocketPeer.WRITE_MODE_BINARY)


func send_move(seq: int, dir_x: float, dir_z: float, yaw: float) -> void:
	var m := PB.MoveInput.new()
	m.seq = seq
	m.dir_x = dir_x
	m.dir_z = dir_z
	m.yaw = yaw
	var msg := PB.ClientMessage.new()
	msg.move = m
	send(msg)


func _process(delta: float) -> void:
	if _ws == null:
		return
	_ws.poll()
	var state := _ws.get_ready_state()

	if state == WebSocketPeer.STATE_OPEN:
		_was_open = true
		if not _hello_sent:
			_send_hello()
		_ping_timer -= delta
		if _ping_timer <= 0.0:
			_ping_timer = PING_INTERVAL
			_send_ping()

	while _ws != null and _ws.get_available_packet_count() > 0:
		_dispatch(_ws.get_packet())

	if _ws != null and state == WebSocketPeer.STATE_CLOSED:
		var reason := _last_error
		if reason.is_empty():
			reason = _ws.get_close_reason()
		if reason.is_empty():
			reason = "Connection closed" if _was_open else "Could not connect"
		_ws = null
		disconnected.emit(reason)


func _send_hello() -> void:
	_hello_sent = true
	var hello := PB.Hello.new()
	hello.token = _token
	hello.protocol_version = PB.ProtocolVersion.PROTOCOL_VERSION_CURRENT
	var msg := PB.ClientMessage.new()
	msg.hello = hello
	send(msg)


func _send_ping() -> void:
	var ping := PB.Ping.new()
	ping.client_time_ms = Time.get_ticks_msec()
	var msg := PB.ClientMessage.new()
	msg.ping = ping
	send(msg)


func _dispatch(bytes: PackedByteArray) -> void:
	var msg := PB.ServerMessage.from_bytes(bytes)
	if msg == null:
		push_warning("Malformed server message (%d bytes)" % bytes.size())
		return
	match msg.which_payload():
		"snapshot":
			snapshot_received.emit(msg.snapshot)
		"welcome":
			welcomed.emit(msg.welcome)
		"pong":
			rtt_ms = Time.get_ticks_msec() - msg.pong.client_time_ms
		"error":
			_last_error = "%s (%s)" % [msg.error.message, msg.error.code]
			push_warning("Server error: " + _last_error)
