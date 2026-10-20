## Entry point: login, character selection, then the world once the server
## welcomes us.
##
## Command-line options (after `--`), handy for testing several clients:
##   --server=http://127.0.0.1:7777
##   --user=alice --password=...   log in (registering first if needed)
##   --character=Lin --model=zuo_mo   play this character, creating it if needed
##   --autoconnect      do all of the above without clicking
##   --window=x,y,w,h   place the window
##   --autowalk         walk in a circle (automated tests)
##   --autofight        target, chase and attack the nearest enemy (automated tests)
##   --cam_yaw=deg      initial camera angle around the player (0 = front view)
##   --screenshot=path  save a screenshot after --screenshot_delay seconds (default 6)
##   --screenshot_on_fate=path  save path_1..3.png right after the first fate events
extends Node

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const World := preload("res://game/world.gd")
const Models := preload("res://game/models.gd")

var _opts := {}
var _ui: CenterContainer
var _login_box: VBoxContainer
var _chars_box: VBoxContainer
var _server_edit: LineEdit
var _user_edit: LineEdit
var _pass_edit: LineEdit
var _char_list: VBoxContainer
var _name_edit: LineEdit
var _model_select: OptionButton
var _status: Label
var _busy := false
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
	_build_ui()
	_show_login()
	if _opts.has("autoconnect"):
		_autoconnect()


func _unhandled_input(event: InputEvent) -> void:
	if _world != null and event is InputEventKey and event.pressed and event.physical_keycode == KEY_ESCAPE:
		Net.close()
		_on_disconnected("Logged out")


# --- Actions -----------------------------------------------------------------

func _server() -> String:
	return _server_edit.text.strip_edges().trim_suffix("/")


func _do_login() -> void:
	if _busy:
		return
	_busy = true
	_status.text = "Logging in..."
	var err: String = await Net.login(_server(), _user_edit.text.strip_edges(), _pass_edit.text)
	_busy = false
	if not err.is_empty():
		_status.text = err
		return
	_status.text = ""
	_show_characters()


func _do_register() -> void:
	if _busy:
		return
	_busy = true
	_status.text = "Creating account..."
	var err: String = await Net.register(_server(), _user_edit.text.strip_edges(), _pass_edit.text)
	_busy = false
	if not err.is_empty():
		_status.text = err
		return
	await _do_login()


func _do_create() -> void:
	if _busy:
		return
	_busy = true
	_status.text = "Creating character..."
	var model: String = Models.keys()[_model_select.selected]
	var err: String = await Net.create_character(_name_edit.text.strip_edges(), model)
	_busy = false
	_status.text = err
	if err.is_empty():
		_show_characters()


func _do_play(character_id: int) -> void:
	if _busy:
		return
	_busy = true
	_status.text = "Entering the world..."
	var err: String = await Net.play(character_id)
	_busy = false
	_status.text = err


func _autoconnect() -> void:
	var user: String = _opts.get("user", "")
	if user.is_empty():
		return
	var err: String = await Net.login(_server(), user, _pass_edit.text)
	if not err.is_empty():
		await Net.register(_server(), user, _pass_edit.text)
		err = await Net.login(_server(), user, _pass_edit.text)
	if not err.is_empty():
		_status.text = err
		return
	var wanted: String = _opts.get("character", "")
	var chosen := _find_character(wanted)
	if chosen.is_empty() and not wanted.is_empty():
		err = await Net.create_character(wanted, _opts.get("model", Models.DEFAULT_MODEL))
		if not err.is_empty():
			_status.text = err
			return
		chosen = _find_character(wanted)
	if chosen.is_empty() and not Net.characters.is_empty():
		chosen = Net.characters[0]
	if chosen.is_empty():
		_show_characters()
		return
	await _do_play(int(chosen["id"]))


func _find_character(char_name: String) -> Dictionary:
	for c in Net.characters:
		if str(c["name"]).to_lower() == char_name.to_lower():
			return c
	return {}


func _on_welcomed(welcome: PB.Welcome) -> void:
	_ui.hide()
	_world = World.new()
	_world.autowalk = _opts.has("autowalk")
	_world.autofight = _opts.has("autofight")
	add_child(_world)
	_world.start(welcome)
	if _opts.has("cam_yaw"):
		_world.set_camera_yaw(deg_to_rad(float(_opts["cam_yaw"])))
	get_window().title = "Convergence — %s" % welcome.character_name
	if _opts.has("screenshot"):
		_take_screenshot(_opts["screenshot"])
	if _opts.has("screenshot_on_fate"):
		Net.fate_received.connect(_on_fate_screenshot)


func _on_disconnected(reason: String) -> void:
	if _world != null:
		_world.queue_free()
		_world = null
	_ui.show()
	if Net.server_url.is_empty():
		_show_login()
	else:
		_show_characters()
	_status.text = reason


var _fate_shots := 0


## Test option: saves <path>_<n>.png shortly after each of the first 3 fate events.
func _on_fate_screenshot(_ev) -> void:
	_fate_shots += 1
	if _fate_shots > 3:
		return
	var n := _fate_shots
	await get_tree().create_timer(1.0).timeout
	var path: String = _opts["screenshot_on_fate"].trim_suffix(".png") + "_%d.png" % n
	get_viewport().get_texture().get_image().save_png(path)
	print("screenshot saved: ", path)


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


# --- UI ----------------------------------------------------------------------

func _show_login() -> void:
	_login_box.show()
	_chars_box.hide()


func _show_characters() -> void:
	_login_box.hide()
	_chars_box.show()
	for child in _char_list.get_children():
		child.queue_free()
	if Net.characters.is_empty():
		_char_list.add_child(_label("No characters yet. Create one below."))
	for c in Net.characters:
		var row := HBoxContainer.new()
		var info := _label("%s  ·  Lv %d  ·  %s" % [c["name"], int(c["level"]), Models.label(c["model"])])
		info.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		row.add_child(info)
		var play := Button.new()
		play.text = "Play"
		var id := int(c["id"])
		play.pressed.connect(func(): _do_play(id))
		row.add_child(play)
		_char_list.add_child(row)


func _build_ui() -> void:
	_ui = CenterContainer.new()
	_ui.set_anchors_preset(Control.PRESET_FULL_RECT)
	add_child(_ui)
	var panel := PanelContainer.new()
	_ui.add_child(panel)
	var margin := MarginContainer.new()
	for side in ["left", "right", "top", "bottom"]:
		margin.add_theme_constant_override("margin_" + side, 20)
	panel.add_child(margin)
	var root := VBoxContainer.new()
	root.custom_minimum_size = Vector2(400, 0)
	root.add_theme_constant_override("separation", 8)
	margin.add_child(root)

	var title := Label.new()
	title.text = "Convergence"
	title.add_theme_font_size_override("font_size", 24)
	root.add_child(title)

	# Login.
	_login_box = VBoxContainer.new()
	_login_box.add_theme_constant_override("separation", 6)
	root.add_child(_login_box)
	_login_box.add_child(_label("Server"))
	_server_edit = LineEdit.new()
	_server_edit.text = _opts.get("server", "http://127.0.0.1:7777")
	_login_box.add_child(_server_edit)
	_login_box.add_child(_label("Username"))
	_user_edit = LineEdit.new()
	_user_edit.text = _opts.get("user", "")
	_login_box.add_child(_user_edit)
	_login_box.add_child(_label("Password"))
	_pass_edit = LineEdit.new()
	_pass_edit.secret = true
	_pass_edit.text = _opts.get("password", "")
	_pass_edit.text_submitted.connect(func(_t): _do_login())
	_login_box.add_child(_pass_edit)
	var buttons := HBoxContainer.new()
	var login := Button.new()
	login.text = "Log in"
	login.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	login.pressed.connect(_do_login)
	buttons.add_child(login)
	var register := Button.new()
	register.text = "Create account"
	register.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	register.pressed.connect(_do_register)
	buttons.add_child(register)
	_login_box.add_child(buttons)

	# Characters.
	_chars_box = VBoxContainer.new()
	_chars_box.add_theme_constant_override("separation", 6)
	root.add_child(_chars_box)
	_chars_box.add_child(_label("Your characters"))
	_char_list = VBoxContainer.new()
	_chars_box.add_child(_char_list)
	_chars_box.add_child(HSeparator.new())
	_chars_box.add_child(_label("New character"))
	_name_edit = LineEdit.new()
	_name_edit.placeholder_text = "Name"
	_name_edit.max_length = 16
	_chars_box.add_child(_name_edit)
	_model_select = OptionButton.new()
	var keys := Models.keys()
	for i in keys.size():
		_model_select.add_item(Models.label(keys[i]), i)
	_model_select.selected = maxi(keys.find(_opts.get("model", Models.DEFAULT_MODEL)), 0)
	_chars_box.add_child(_model_select)
	var create := Button.new()
	create.text = "Create character"
	create.pressed.connect(_do_create)
	_chars_box.add_child(create)

	_status = Label.new()
	_status.autowrap_mode = TextServer.AUTOWRAP_WORD
	_status.custom_minimum_size = Vector2(400, 0)
	root.add_child(_status)


func _label(text: String) -> Label:
	var l := Label.new()
	l.text = text
	return l
