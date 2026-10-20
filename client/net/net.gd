## Connection to the game server (autoload "Net").
##
## Flow: login() or register() over HTTP, then create_character() if needed,
## then play() which gets a ticket, opens the WebSocket and sends Hello.
## Server messages arrive as signals.
extends Node

signal welcomed(welcome: PB.Welcome)
signal snapshot_received(snapshot: PB.Snapshot)
signal skill_rejected(skill_id: int, reason: String)
signal fate_received(event: PB.FateEvent)
signal disconnected(reason: String)

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const PING_INTERVAL := 2.0

var rtt_ms := 0.0
var server_url := ""
var characters: Array = []  # [{id, name, model, level, zone_id}]

var _session := ""
var _ws: WebSocketPeer
var _ticket := ""
var _hello_sent := false
var _was_open := false
var _ping_timer := 0.0
var _last_error := ""


## Each returns "" on success or a message to show the player.
func register(server: String, username: String, password: String) -> String:
	var res := await _request(server, HTTPClient.METHOD_POST, "/api/v1/register",
			{"username": username, "password": password})
	return res["error"]


func login(server: String, username: String, password: String) -> String:
	var res := await _request(server, HTTPClient.METHOD_POST, "/api/v1/login",
			{"username": username, "password": password})
	if res["error"].is_empty():
		server_url = server
		_session = res["data"]["session"]
		characters = res["data"]["characters"]
	return res["error"]


func create_character(char_name: String, model: String, class_key: String) -> String:
	var res := await _request(server_url, HTTPClient.METHOD_POST, "/api/v1/characters",
			{"name": char_name, "model": model, "class": class_key})
	if res["error"].is_empty():
		characters.append(res["data"]["character"])
	return res["error"]


func play(character_id: int) -> String:
	close()
	var res := await _request(server_url, HTTPClient.METHOD_POST, "/api/v1/play", {"character_id": character_id})
	if not res["error"].is_empty():
		return res["error"]
	_ticket = res["data"]["ticket"]
	var ws_url := server_url.replace("https://", "wss://").replace("http://", "ws://") + str(res["data"]["ws_path"])
	_ws = WebSocketPeer.new()
	_ws.inbound_buffer_size = 1 << 20
	var err := _ws.connect_to_url(ws_url)
	if err != OK:
		_ws = null
		return "WebSocket connection failed (%s)" % error_string(err)
	_hello_sent = false
	_was_open = false
	_last_error = ""
	return ""


## Returns {"error": String, "data": Variant}.
func _request(server: String, method: int, path: String, body: Dictionary) -> Dictionary:
	var http := HTTPRequest.new()
	add_child(http)
	var headers := PackedStringArray(["Content-Type: application/json"])
	if not _session.is_empty():
		headers.append("Authorization: Bearer " + _session)
	var err := http.request(server + path, headers, method, JSON.stringify(body))
	if err != OK:
		http.queue_free()
		return {"error": "Request failed (%s)" % error_string(err), "data": null}
	var res: Array = await http.request_completed
	http.queue_free()
	if res[0] != HTTPRequest.RESULT_SUCCESS:
		return {"error": "Cannot reach the server at %s" % server, "data": null}
	var data = JSON.parse_string((res[3] as PackedByteArray).get_string_from_utf8())
	if res[1] >= 300:
		var msg := "Server returned HTTP %d" % res[1]
		if typeof(data) == TYPE_DICTIONARY and data.has("error"):
			msg = data["error"]
		return {"error": msg, "data": data}
	return {"error": "", "data": data}


func close() -> void:
	if _ws != null:
		_ws.close()
		_ws = null


func is_open() -> bool:
	return _ws != null and _ws.get_ready_state() == WebSocketPeer.STATE_OPEN


func send(msg: PB.ClientMessage) -> void:
	if is_open():
		_ws.send(msg.to_bytes(), WebSocketPeer.WRITE_MODE_BINARY)


func send_move(seq: int, dir_x: float, dir_z: float, yaw: float, buttons := 0, lock_target := 0) -> void:
	var m := PB.MoveInput.new()
	m.seq = seq
	m.dir_x = dir_x
	m.dir_z = dir_z
	m.yaw = yaw
	m.buttons = buttons
	m.lock_target = lock_target
	var msg := PB.ClientMessage.new()
	msg.move = m
	send(msg)


func send_skill(skill_id: int, target_id: int) -> void:
	var u := PB.UseSkill.new()
	u.skill_id = skill_id
	u.target_id = target_id
	var msg := PB.ClientMessage.new()
	msg.use_skill = u
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
	hello.token = _ticket
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
		"skill_rejected":
			skill_rejected.emit(msg.skill_rejected.skill_id, msg.skill_rejected.reason)
		"fate":
			fate_received.emit(msg.fate)
		"pong":
			rtt_ms = Time.get_ticks_msec() - msg.pong.client_time_ms
		"error":
			_last_error = "%s (%s)" % [msg.error.message, msg.error.code]
			push_warning("Server error: " + _last_error)
