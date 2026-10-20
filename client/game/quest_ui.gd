## Quest dialog (talking to a quest giver) and the quest tracker.
extends Control

const PB := preload("res://gen/convergence/v1/world.pb.gd")

signal action_requested(quest: String, action: String, choice: String, npc_id: int)
signal closed

var _dialog: PanelContainer
var _dialog_box: VBoxContainer
var _tracker: Label
var _npc_id := 0


func _ready() -> void:
	set_anchors_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_IGNORE

	_dialog = PanelContainer.new()
	_dialog.set_anchors_and_offsets_preset(Control.PRESET_CENTER)
	_dialog.grow_horizontal = Control.GROW_DIRECTION_BOTH
	_dialog.grow_vertical = Control.GROW_DIRECTION_BOTH
	var style := StyleBoxFlat.new()
	style.bg_color = Color(0.07, 0.06, 0.05, 0.94)
	style.border_color = Color(0.7, 0.58, 0.3)
	style.set_border_width_all(2)
	style.set_corner_radius_all(8)
	style.set_content_margin_all(18)
	_dialog.add_theme_stylebox_override("panel", style)
	_dialog_box = VBoxContainer.new()
	_dialog_box.custom_minimum_size = Vector2(560, 0)
	_dialog_box.add_theme_constant_override("separation", 8)
	_dialog.add_child(_dialog_box)
	_dialog.hide()
	add_child(_dialog)

	_tracker = Label.new()
	_tracker.set_anchors_and_offsets_preset(Control.PRESET_TOP_RIGHT)
	_tracker.grow_horizontal = Control.GROW_DIRECTION_BEGIN
	_tracker.offset_top = 60
	_tracker.offset_right = -14
	_tracker.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
	_tracker.add_theme_font_size_override("font_size", 13)
	_tracker.add_theme_color_override("font_outline_color", Color.BLACK)
	_tracker.add_theme_constant_override("outline_size", 5)
	add_child(_tracker)


func is_open() -> bool:
	return _dialog.visible


func close() -> void:
	_dialog.hide()
	closed.emit()


func show_dialog(d: PB.QuestDialog) -> void:
	_npc_id = d.npc_id
	for c in _dialog_box.get_children():
		c.queue_free()
	_title(d.npc_name, 20, Color(1.0, 0.85, 0.45))
	_text(d.reply if not d.reply.is_empty() else d.greeting, Color(0.92, 0.9, 0.85))
	for q in d.ready:
		_dialog_box.add_child(HSeparator.new())
		_title("%s  (ready)" % q.name, 16, Color(0.6, 1.0, 0.6))
		_text(q.text, Color(0.85, 0.85, 0.85))
		if q.choices.is_empty():
			_button("Complete", func(): action_requested.emit(q.key, "complete", "", _npc_id))
		for ch in q.choices:
			var key: String = ch.key
			_button(ch.text, func(): action_requested.emit(q.key, "complete", key, _npc_id))
	for q in d.offers:
		_dialog_box.add_child(HSeparator.new())
		_title(q.name, 16, Color(1.0, 0.85, 0.45))
		_text(q.text, Color(0.85, 0.85, 0.85))
		_text("Task: " + q.summary, Color(0.75, 0.8, 0.95))
		_button("Accept", func(): action_requested.emit(q.key, "accept", "", _npc_id))
	if d.offers.is_empty() and d.ready.is_empty() and d.reply.is_empty():
		_text("(Nothing more for now.)", Color(0.6, 0.6, 0.6))
	_dialog_box.add_child(HSeparator.new())
	_button("Farewell", close)
	_dialog.show()
	# Center once the new content has been laid out.
	await get_tree().process_frame
	_dialog.reset_size()
	_dialog.position = (get_viewport_rect().size - _dialog.size) / 2.0


func show_log(log: PB.QuestLog) -> void:
	var lines := PackedStringArray()
	for q in log.quests:
		if q.state == "done":
			continue
		lines.append(("%s  —  return to the giver" if q.state == "ready" else "%s") % q.name)
		for o in q.objectives:
			lines.append("   %s  %d/%d" % [o.text, o.progress, o.target])
	_tracker.text = "\n".join(lines)


func _title(t: String, size: int, color: Color) -> void:
	var l := Label.new()
	l.text = t
	l.add_theme_font_size_override("font_size", size)
	l.modulate = color
	_dialog_box.add_child(l)


func _text(t: String, color: Color) -> void:
	var l := Label.new()
	l.text = t
	l.autowrap_mode = TextServer.AUTOWRAP_WORD
	l.custom_minimum_size = Vector2(540, 0)
	l.modulate = color
	_dialog_box.add_child(l)


func _button(t: String, fn: Callable) -> void:
	var b := Button.new()
	b.text = t
	b.focus_mode = Control.FOCUS_NONE
	b.pressed.connect(fn)
	_dialog_box.add_child(b)
