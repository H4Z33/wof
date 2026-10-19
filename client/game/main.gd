## Entry point: login screen, then the world once the server welcomes us.
##
## Command-line options (after `--`), handy for testing several clients:
##   --server=http://127.0.0.1:7777  --name=Alice  --model=zuo_mo  --autoconnect
##   --window=x,y,w,h   place the window
##   --autowalk         walk in a circle (automated tests)
##   --cam_yaw=deg      initial camera angle around the player (180 = front view)
##   --screenshot=path  save a screenshot after a few seconds in the world
extends Node

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const World := preload("res://game/world.gd")
const Models := preload("res://game/models.gd")

var _opts := {}
var _login_ui: Control
var _server_edit: LineEdit
var _name_edit: LineEdit
var _model_select: OptionButton
var _connect_button: Button
var _status: Label
var _world: Node3D


func _ready() -> void:
	_opts = _parse_args()
	if _opts.has("window"):
		var r: PackedStringArray = _opts["window"].split(",")
		if r.size() == 4:
			get_window().position = Vector2i(int(r[0]), int(r[1]))
			get_window().size = Vector2i(int(r[2]), int(r[3]))
	Net.welcomed.connect(_on_welcomed)
	Net.disconnected.connect(_on_disconnected)
	_build_login()
	if _opts.has("autoconnect"):
		_on_connect_pressed()


func _unhandled_input(event: InputEvent) -> void:
	if _world != null and event is InputEventKey and event.pressed and event.physical_keycode == KEY_ESCAPE:
		Net.close()
		_on_disconnected("Disconnected")


func _on_connect_pressed() -> void:
	_connect_button.disabled = true
	_status.text = "Connecting..."
	var model: String = Models.keys()[_model_select.selected]
	var err: String = await Net.login(_server_edit.text.strip_edges().trim_suffix("/"), _name_edit.text.strip_edges(), model)
	if not err.is_empty():
		_status.text = err
		_connect_button.disabled = false


func _on_welcomed(welcome: PB.Welcome) -> void:
	_login_ui.hide()
	_world = World.new()
	_world.autowalk = _opts.has("autowalk")
	if _opts.has("cam_yaw"):
		_world.set_camera_yaw(deg_to_rad(float(_opts["cam_yaw"])))
	add_child(_world)
	_world.start(welcome)
	get_window().title = "Convergence — %s" % _name_edit.text
	if _opts.has("screenshot"):
		_take_screenshot(_opts["screenshot"])


func _on_disconnected(reason: String) -> void:
	if _world != null:
		_world.queue_free()
		_world = null
	_login_ui.show()
	_connect_button.disabled = false
	_status.text = reason


func _take_screenshot(path: String) -> void:
	await get_tree().create_timer(float(_opts.get("screenshot_delay", "6"))).timeout
	var img := get_viewport().get_texture().get_image()
	img.save_png(path)
	print("screenshot saved: ", path)


func _parse_args() -> Dictionary:
	var opts := {}
	for arg in OS.get_cmdline_user_args():
		if not arg.begins_with("--"):
			continue
		var kv := arg.substr(2).split("=", true, 1)
		opts[kv[0]] = kv[1] if kv.size() > 1 else "true"
	return opts


func _build_login() -> void:
	_login_ui = CenterContainer.new()
	_login_ui.set_anchors_preset(Control.PRESET_FULL_RECT)
	add_child(_login_ui)

	var panel := PanelContainer.new()
	_login_ui.add_child(panel)
	var box := VBoxContainer.new()
	box.custom_minimum_size = Vector2(360, 0)
	box.add_theme_constant_override("separation", 8)
	var margin := MarginContainer.new()
	for side in ["left", "right", "top", "bottom"]:
		margin.add_theme_constant_override("margin_" + side, 20)
	panel.add_child(margin)
	margin.add_child(box)

	var title := Label.new()
	title.text = "Convergence — dev login"
	title.add_theme_font_size_override("font_size", 22)
	box.add_child(title)

	box.add_child(_label("Server"))
	_server_edit = LineEdit.new()
	_server_edit.text = _opts.get("server", "http://127.0.0.1:7777")
	box.add_child(_server_edit)

	box.add_child(_label("Name"))
	_name_edit = LineEdit.new()
	_name_edit.text = _opts.get("name", "Wanderer%d" % randi_range(100, 999))
	_name_edit.max_length = 16
	box.add_child(_name_edit)

	box.add_child(_label("Character"))
	_model_select = OptionButton.new()
	var keys := Models.keys()
	for i in keys.size():
		_model_select.add_item(Models.label(keys[i]), i)
	var wanted := keys.find(_opts.get("model", Models.DEFAULT_MODEL))
	_model_select.selected = maxi(wanted, 0)
	box.add_child(_model_select)

	_connect_button = Button.new()
	_connect_button.text = "Enter world"
	_connect_button.pressed.connect(_on_connect_pressed)
	box.add_child(_connect_button)
	_name_edit.text_submitted.connect(func(_t): _on_connect_pressed())

	_status = Label.new()
	_status.autowrap_mode = TextServer.AUTOWRAP_WORD
	box.add_child(_status)


func _label(text: String) -> Label:
	var l := Label.new()
	l.text = text
	return l
