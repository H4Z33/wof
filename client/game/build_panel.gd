## Character build panel (C): discipline experience, attribute ranks to
## buy, and the specialization choice once unlocked.
extends PanelContainer

const PB := preload("res://gen/convergence/v1/world.pb.gd")

signal raise_requested(key: String)
signal spec_chosen(key: String)

var _box: VBoxContainer


func _ready() -> void:
	set_anchors_and_offsets_preset(Control.PRESET_CENTER_LEFT)
	offset_left = 16
	custom_minimum_size = Vector2(380, 0)
	var style := StyleBoxFlat.new()
	style.bg_color = Color(0.06, 0.06, 0.09, 0.9)
	style.set_corner_radius_all(6)
	style.set_content_margin_all(14)
	add_theme_stylebox_override("panel", style)
	_box = VBoxContainer.new()
	_box.add_theme_constant_override("separation", 4)
	add_child(_box)
	hide()


func show_progression(pr: PB.Progression) -> void:
	for child in _box.get_children():
		child.queue_free()
	var title := Label.new()
	title.text = "Disciplines"
	title.add_theme_font_size_override("font_size", 18)
	_box.add_child(title)

	var xp := {}
	for d in pr.disciplines:
		xp[d.key] = d.xp
		var l := Label.new()
		l.text = "%s%s   ·   %d exp to spend   ·   %d ranks" % [d.name, "  (class)" if d.affinity else "", d.xp, d.ranks]
		l.modulate = Color(1.0, 0.9, 0.6) if d.affinity else Color(0.85, 0.85, 0.9)
		_box.add_child(l)
	_box.add_child(HSeparator.new())

	for a in pr.attributes:
		var row := HBoxContainer.new()
		var info := Label.new()
		info.text = "%s  %d/%d" % [a.name, a.rank, a.cap]
		info.tooltip_text = a.description
		info.mouse_filter = Control.MOUSE_FILTER_PASS
		info.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		row.add_child(info)
		var desc := Label.new()
		desc.text = a.description
		desc.add_theme_font_size_override("font_size", 11)
		desc.modulate = Color(0.7, 0.7, 0.75)
		desc.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		row.add_child(desc)
		var b := Button.new()
		var maxed: bool = a.rank >= a.cap
		b.text = "max" if maxed else "+ %d" % a.next_cost
		b.disabled = maxed or xp.get(a.discipline, 0) < a.next_cost
		b.focus_mode = Control.FOCUS_NONE
		var key: String = a.key
		b.pressed.connect(func(): raise_requested.emit(key))
		row.add_child(b)
		_box.add_child(row)

	_box.add_child(HSeparator.new())
	var spec := Label.new()
	spec.autowrap_mode = TextServer.AUTOWRAP_WORD
	if not pr.spec.is_empty():
		spec.text = "Specialization: %s" % pr.spec.capitalize()
	elif pr.spec_options.is_empty():
		spec.text = "Specialization unlocks at %d ranks in your class discipline." % pr.spec_unlock_ranks
	else:
		spec.text = "Choose your path (permanent):"
	_box.add_child(spec)
	for o in pr.spec_options:
		var b := Button.new()
		b.text = "%s  —  %s (new skill: %s)" % [o.name, o.description, o.skill_name]
		b.autowrap_mode = TextServer.AUTOWRAP_WORD
		b.custom_minimum_size = Vector2(350, 0)
		var key: String = o.key
		b.pressed.connect(func(): spec_chosen.emit(key))
		_box.add_child(b)
