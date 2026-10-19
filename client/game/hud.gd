## In-world HUD: player bars, target frame, skill bar, messages, death screen.
extends CanvasLayer

const PB := preload("res://gen/convergence/v1/world.pb.gd")

var debug_label: Label

var _target_panel: PanelContainer
var _target_name: Label
var _target_hp: ProgressBar
var _target_hp_text: Label
var _hp_bar: ProgressBar
var _hp_text: Label
var _exp_bar: ProgressBar
var _level_label: Label
var _message: Label
var _message_tween: Tween
var _death: Label
var _skill_buttons := {}  # skill id -> Button
var _skill_names := {}

signal skill_pressed(skill_id: int)


func _ready() -> void:
	debug_label = Label.new()
	debug_label.set_anchors_and_offsets_preset(Control.PRESET_BOTTOM_LEFT)
	debug_label.grow_vertical = Control.GROW_DIRECTION_BEGIN
	debug_label.offset_left = 12
	debug_label.offset_bottom = -10
	debug_label.add_theme_font_size_override("font_size", 12)
	debug_label.modulate = Color(1, 1, 1, 0.75)
	_outline(debug_label)
	add_child(debug_label)

	# Target frame, top center.
	_target_panel = PanelContainer.new()
	_target_panel.set_anchors_and_offsets_preset(Control.PRESET_CENTER_TOP)
	_target_panel.offset_top = 12
	_target_panel.custom_minimum_size = Vector2(300, 0)
	_target_panel.position.x -= 150
	var tbox := VBoxContainer.new()
	_target_panel.add_child(tbox)
	_target_name = Label.new()
	_target_name.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	tbox.add_child(_target_name)
	_target_hp = _bar(Color(0.75, 0.18, 0.15), Vector2(280, 16))
	tbox.add_child(_target_hp)
	_target_hp_text = Label.new()
	_target_hp_text.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_target_hp_text.add_theme_font_size_override("font_size", 12)
	tbox.add_child(_target_hp_text)
	_target_panel.hide()
	add_child(_target_panel)

	# Player bars and skills, bottom center.
	var bottom := VBoxContainer.new()
	bottom.set_anchors_and_offsets_preset(Control.PRESET_CENTER_BOTTOM)
	bottom.grow_vertical = Control.GROW_DIRECTION_BEGIN
	bottom.grow_horizontal = Control.GROW_DIRECTION_BOTH
	bottom.offset_bottom = -14
	bottom.alignment = BoxContainer.ALIGNMENT_END
	bottom.add_theme_constant_override("separation", 4)
	add_child(bottom)

	_message = Label.new()
	_message.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_message.add_theme_font_size_override("font_size", 20)
	_outline(_message)
	bottom.add_child(_message)

	_level_label = Label.new()
	_level_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_outline(_level_label)
	bottom.add_child(_level_label)
	_exp_bar = _bar(Color(0.85, 0.7, 0.25), Vector2(360, 6))
	bottom.add_child(_exp_bar)
	var hp_stack := Control.new()
	hp_stack.custom_minimum_size = Vector2(360, 18)
	_hp_bar = _bar(Color(0.25, 0.65, 0.3), Vector2(360, 18))
	hp_stack.add_child(_hp_bar)
	_hp_text = Label.new()
	_hp_text.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	_hp_text.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_hp_text.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	_hp_text.add_theme_font_size_override("font_size", 12)
	_outline(_hp_text)
	hp_stack.add_child(_hp_text)
	bottom.add_child(hp_stack)

	var skills := HBoxContainer.new()
	skills.alignment = BoxContainer.ALIGNMENT_CENTER
	skills.name = "Skills"
	bottom.add_child(skills)

	_death = Label.new()
	_death.set_anchors_and_offsets_preset(Control.PRESET_CENTER)
	_death.grow_horizontal = Control.GROW_DIRECTION_BOTH
	_death.add_theme_font_size_override("font_size", 32)
	_death.add_theme_color_override("font_color", Color(0.95, 0.4, 0.35))
	_outline(_death)
	_death.hide()
	add_child(_death)


func set_skills(skills: Array) -> void:
	var box: HBoxContainer = find_child("Skills", true, false)
	for i in skills.size():
		var s: PB.SkillInfo = skills[i]
		var b := Button.new()
		b.custom_minimum_size = Vector2(110, 44)
		b.focus_mode = Control.FOCUS_NONE
		b.pressed.connect(func(): skill_pressed.emit(s.id))
		box.add_child(b)
		_skill_buttons[s.id] = b
		_skill_names[s.id] = "%d  %s" % [i + 1, s.name]
		b.text = _skill_names[s.id]


## remaining: skill id -> seconds left on cooldown.
func update_cooldowns(remaining: Dictionary) -> void:
	for id in _skill_buttons:
		var b: Button = _skill_buttons[id]
		var left: float = remaining.get(id, 0.0)
		b.disabled = left > 0.0
		b.text = _skill_names[id] if left <= 0.0 else "%s  %.1f" % [_skill_names[id], left]


func set_status(s: PB.PlayerStatus) -> void:
	_hp_bar.max_value = maxi(s.max_hp, 1)
	_hp_bar.value = s.hp
	_hp_text.text = "%d / %d" % [s.hp, s.max_hp]
	_exp_bar.max_value = maxi(s.exp_to_next, 1)
	_exp_bar.value = s.exp
	_level_label.text = "Level %d   ·   %d / %d exp" % [s.level, s.exp, s.exp_to_next]
	if s.hp == 0:
		_death.text = "You died. Respawning in %d..." % ceili(s.respawn_in_ms / 1000.0)
		_death.show()
	else:
		_death.hide()


## target: Character or null.
func set_target(target) -> void:
	if target == null or not is_instance_valid(target):
		_target_panel.hide()
		return
	_target_panel.show()
	var lvl: String = "  Lv %d" % target.level if target.is_npc() else ""
	_target_name.text = target.display_name + lvl
	_target_hp.max_value = maxi(target.max_hp, 1)
	_target_hp.value = target.hp
	_target_hp_text.text = "%d / %d" % [target.hp, target.max_hp] if target.hp > 0 else "Dead"


func show_message(text: String, color := Color.WHITE) -> void:
	_message.text = text
	_message.modulate = color
	if _message_tween != null:
		_message_tween.kill()
	_message_tween = create_tween()
	_message_tween.tween_interval(1.5)
	_message_tween.tween_property(_message, "modulate:a", 0.0, 0.6)


func _bar(color: Color, size: Vector2) -> ProgressBar:
	var bar := ProgressBar.new()
	bar.custom_minimum_size = size
	bar.show_percentage = false
	var fill := StyleBoxFlat.new()
	fill.bg_color = color
	var bg := StyleBoxFlat.new()
	bg.bg_color = Color(0, 0, 0, 0.55)
	bar.add_theme_stylebox_override("fill", fill)
	bar.add_theme_stylebox_override("background", bg)
	return bar


func _outline(l: Label) -> void:
	l.add_theme_color_override("font_outline_color", Color.BLACK)
	l.add_theme_constant_override("outline_size", 6)
