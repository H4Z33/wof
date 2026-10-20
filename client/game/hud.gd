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
var _res_bar: ProgressBar
var _res_text: Label
var _res_name := "" 
var _exp_bar: ProgressBar
var _level_label: Label
var _message: Label
var _message_tween: Tween
var _death: Label
var _skill_buttons := {}  # skill id -> Button
var _skill_names := {}
var _stats_label: Label
var _fate_panel: PanelContainer
var _fate_title: Label
var _fate_text: Label
var _fate_effect: Label
var _fate_tween: Tween
var _inventory_panel: PanelContainer
var _inventory_list: Label

signal skill_pressed(skill_id: int)


func _ready() -> void:
	debug_label = Label.new()
	debug_label.set_anchors_and_offsets_preset(Control.PRESET_TOP_RIGHT)
	debug_label.grow_horizontal = Control.GROW_DIRECTION_BEGIN
	debug_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
	debug_label.offset_right = -12
	debug_label.offset_top = 8
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

	_stats_label = Label.new()
	_stats_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_stats_label.add_theme_font_size_override("font_size", 13)
	_stats_label.modulate = Color(0.85, 0.85, 1.0)
	_outline(_stats_label)
	bottom.add_child(_stats_label)

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
	var res_stack := Control.new()
	res_stack.custom_minimum_size = Vector2(360, 12)
	_res_bar = _bar(Color(0.4, 0.6, 1.0), Vector2(360, 12))
	res_stack.add_child(_res_bar)
	_res_text = Label.new()
	_res_text.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	_res_text.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_res_text.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	_res_text.add_theme_font_size_override("font_size", 10)
	_outline(_res_text)
	res_stack.add_child(_res_text)
	bottom.add_child(res_stack)

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

	# Fate banner, upper middle of the screen.
	_fate_panel = PanelContainer.new()
	_fate_panel.set_anchors_and_offsets_preset(Control.PRESET_CENTER_TOP)
	_fate_panel.grow_horizontal = Control.GROW_DIRECTION_BOTH
	_fate_panel.offset_top = 110
	var fstyle := StyleBoxFlat.new()
	fstyle.bg_color = Color(0.05, 0.05, 0.08, 0.78)
	fstyle.set_corner_radius_all(6)
	fstyle.set_content_margin_all(14)
	_fate_panel.add_theme_stylebox_override("panel", fstyle)
	var fbox := VBoxContainer.new()
	fbox.custom_minimum_size = Vector2(520, 0)
	_fate_panel.add_child(fbox)
	_fate_title = Label.new()
	_fate_title.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_fate_title.add_theme_font_size_override("font_size", 14)
	fbox.add_child(_fate_title)
	_fate_text = Label.new()
	_fate_text.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_fate_text.autowrap_mode = TextServer.AUTOWRAP_WORD
	_fate_text.add_theme_font_size_override("font_size", 19)
	fbox.add_child(_fate_text)
	_fate_effect = Label.new()
	_fate_effect.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_fate_effect.add_theme_font_size_override("font_size", 14)
	fbox.add_child(_fate_effect)
	_fate_panel.hide()
	add_child(_fate_panel)

	# Inventory (I), right side.
	_inventory_panel = PanelContainer.new()
	_inventory_panel.set_anchors_and_offsets_preset(Control.PRESET_CENTER_RIGHT)
	_inventory_panel.grow_horizontal = Control.GROW_DIRECTION_BEGIN
	_inventory_panel.offset_right = -14
	var ibox := VBoxContainer.new()
	ibox.custom_minimum_size = Vector2(240, 0)
	_inventory_panel.add_child(ibox)
	var ititle := Label.new()
	ititle.text = "Inventory"
	ititle.add_theme_font_size_override("font_size", 16)
	ibox.add_child(ititle)
	_inventory_list = Label.new()
	_inventory_list.text = "Empty"
	ibox.add_child(_inventory_list)
	_inventory_panel.hide()
	add_child(_inventory_panel)


func set_skills(skills: Array) -> void:
	var box: HBoxContainer = find_child("Skills", true, false)
	for i in skills.size():
		var s: PB.SkillInfo = skills[i]
		var b := Button.new()
		b.custom_minimum_size = Vector2(130, 44)
		b.tooltip_text = "%s
%s%s" % [s.name, s.description, ("
Cost: %d" % s.cost) if s.cost > 0 else ""]
		b.focus_mode = Control.FOCUS_NONE
		b.pressed.connect(func(): skill_pressed.emit(s.id))
		box.add_child(b)
		_skill_buttons[s.id] = b
		_skill_names[s.id] = "%d  %s" % [i + 1, s.name]
		b.text = _skill_names[s.id]


## remaining: skill id -> seconds left on cooldown; affordable: skill id -> bool.
func update_cooldowns(remaining: Dictionary, affordable := {}) -> void:
	for id in _skill_buttons:
		var b: Button = _skill_buttons[id]
		var left: float = remaining.get(id, 0.0)
		b.disabled = left > 0.0 or not affordable.get(id, true)
		b.text = _skill_names[id] if left <= 0.0 else "%s  %.1f" % [_skill_names[id], left]


func set_status(s: PB.PlayerStatus) -> void:
	_hp_bar.max_value = maxi(s.max_hp, 1)
	_hp_bar.value = s.hp
	_hp_text.text = "%d / %d" % [s.hp, s.max_hp]
	_exp_bar.max_value = maxi(s.exp_to_next, 1)
	_exp_bar.value = s.exp
	_level_label.text = "Level %d   ·   %d / %d exp" % [s.level, s.exp, s.exp_to_next]
	_stats_label.text = "Luck %d   ·   Karma %+d" % [s.luck, s.karma]
	_res_bar.max_value = maxi(s.resource_max, 1)
	_res_bar.value = s.resource
	_res_text.text = "%s %d / %d" % [_res_name, s.resource, s.resource_max]
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
	var fx: PackedStringArray = target.effect_names()
	_target_name.text = target.display_name + lvl + ("   [%s]" % ", ".join(fx) if not fx.is_empty() else "")
	_target_hp.max_value = maxi(target.max_hp, 1)
	_target_hp.value = target.hp
	_target_hp_text.text = "%d / %d" % [target.hp, target.max_hp] if target.hp > 0 else "Dead"


## Shows a Fate event: who would read it as fortune or misfortune, the
## narration, and what actually changed.
func show_fate(ev: PB.FateEvent, effect_text: String) -> void:
	var fortune := ev.omen == PB.FateOmen.FATE_OMEN_FORTUNE
	var strength: String = ["", "", "Great ", "Heavenly "][clampi(ev.magnitude, 0, 3)]
	_fate_title.text = "✦ %s%s ✦" % [strength, "Fortune" if fortune else "Misfortune"]
	_fate_title.modulate = Color(1.0, 0.82, 0.35) if fortune else Color(0.75, 0.55, 1.0)
	_fate_text.text = ev.narration
	_fate_effect.text = effect_text
	_fate_effect.modulate = _fate_title.modulate
	_fate_panel.modulate.a = 0.0
	_fate_panel.show()
	if _fate_tween != null:
		_fate_tween.kill()
	_fate_tween = create_tween()
	_fate_tween.tween_property(_fate_panel, "modulate:a", 1.0, 0.4)
	_fate_tween.tween_interval(6.0)
	_fate_tween.tween_property(_fate_panel, "modulate:a", 0.0, 1.0)
	_fate_tween.tween_callback(_fate_panel.hide)


## Shows the class resource bar in the class's color.
func set_class(info: PB.ClassInfo) -> void:
	if info == null:
		_res_bar.get_parent().hide()
		return
	_res_name = info.resource_name
	var fill := StyleBoxFlat.new()
	fill.bg_color = Color.html(info.resource_color) if info.resource_color.begins_with("#") else Color(0.4, 0.6, 1.0)
	_res_bar.add_theme_stylebox_override("fill", fill)


func set_inventory(items: Array) -> void:
	var lines := PackedStringArray()
	for it in items:
		lines.append("%s  ×%d" % [it.name, it.quantity])
	_inventory_list.text = "\n".join(lines) if not lines.is_empty() else "Empty"


func toggle_inventory() -> void:
	_inventory_panel.visible = not _inventory_panel.visible


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
