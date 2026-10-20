## A replicated entity: model, animation, nameplate, health and interpolation.
##
## Remote entities render ~100 ms in the past, interpolating between buffered
## server states. The local player is positioned by the world's prediction
## code through set_predicted().
extends Node3D

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const Models := preload("res://game/models.gd")
const ContentDB := preload("res://game/content_db.gd")
# Godot's glTF importer strips the "_Loop" suffix and marks those clips as looping.
const ANIM_IDLE := "Idle"
const ANIM_MOVE := "Jog_Fwd"
const ANIM_HIT := "Hit_Chest"
const ANIM_DEATH := "Death01"
const ANIM_BLEND := 0.2
const MAX_BUFFER := 40
## Physics layer used for click targeting.
const PICK_LAYER := 2

var entity_id := 0
var is_local := false
var class_key := ""
# Current PB.ActionState (swing, block, roll, NPC wind-up...).
var action := 0
var _action_key := -1
# NPC attack look, from EntitySpawn.
var attack_anim := ""
var attack_vfx := ""
var attack_projectile := false
var kind := 0  # PB.EntityKind
var display_name := ""
var level := 1
var hp := 1
var max_hp := 1

var _player: AnimationPlayer
var _moving := false
var _oneshot := false
var _plate: Label3D
# Each entry: {tick: float, pos: Vector3, yaw: float, moving: bool}
var _buffer: Array[Dictionary] = []


func setup(spawn: PB.EntitySpawn, local: bool) -> void:
	entity_id = spawn.id
	is_local = local
	kind = spawn.kind
	display_name = spawn.name
	class_key = spawn.class_key
	level = maxi(spawn.level, 1)
	name = "Entity_%d" % entity_id
	if kind == PB.EntityKind.ENTITY_KIND_OBJECT:
		_build_object()
		return
	var parts := Models.instantiate(spawn.model)
	add_child(parts[0])
	_player = parts[1]
	if _player != null:
		_player.animation_finished.connect(_on_animation_finished)
	var floating := (parts[0] as Node).get_node_or_null("Body") as Node3D
	if floating != null:  # procedural spirits float
		var tw := floating.create_tween().set_loops()
		tw.tween_property(floating, "position:y", 1.55, 1.2).set_trans(Tween.TRANS_SINE)
		tw.tween_property(floating, "position:y", 1.3, 1.2).set_trans(Tween.TRANS_SINE)
	attack_anim = spawn.attack_anim
	attack_vfx = spawn.attack_vfx
	attack_projectile = spawn.attack_projectile

	var body := StaticBody3D.new()
	body.collision_layer = PICK_LAYER
	body.collision_mask = 0
	body.set_meta("entity_id", entity_id)
	var shape := CollisionShape3D.new()
	var capsule := CapsuleShape3D.new()
	capsule.radius = 0.45
	capsule.height = 1.9
	shape.shape = capsule
	shape.position = Vector3(0, 0.95, 0)
	body.add_child(shape)
	add_child(body)

	_plate = Label3D.new()
	_plate.billboard = BaseMaterial3D.BILLBOARD_ENABLED
	_plate.position = Vector3(0, 2.15, 0)
	_plate.font_size = 44
	_plate.pixel_size = 0.005
	_plate.outline_size = 12
	_plate.no_depth_test = true
	add_child(_plate)
	_update_plate()

	if spawn.state != null:
		set_health(spawn.state.hp, spawn.state.max_hp)
	if hp > 0:
		_play(ANIM_IDLE)


func is_npc() -> bool:
	return kind == PB.EntityKind.ENTITY_KIND_NPC


func is_object() -> bool:
	return kind == PB.EntityKind.ENTITY_KIND_OBJECT


## A Fate hidden cache: a hovering, pulsing orb of light.
func _build_object() -> void:
	var orb := MeshInstance3D.new()
	var sphere := SphereMesh.new()
	sphere.radius = 0.3
	sphere.height = 0.6
	orb.mesh = sphere
	var mat := StandardMaterial3D.new()
	mat.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	mat.albedo_color = Color(1.0, 0.85, 0.35)
	mat.emission_enabled = true
	mat.emission = Color(1.0, 0.8, 0.3)
	orb.material_override = mat
	orb.position = Vector3(0, 0.9, 0)
	add_child(orb)
	var light := OmniLight3D.new()
	light.light_color = Color(1.0, 0.8, 0.4)
	light.light_energy = 2.0
	light.omni_range = 5.0
	light.position = orb.position
	add_child(light)
	var tw := orb.create_tween().set_loops()
	tw.tween_property(orb, "position:y", 1.2, 1.0).set_trans(Tween.TRANS_SINE)
	tw.tween_property(orb, "position:y", 0.9, 1.0).set_trans(Tween.TRANS_SINE)
	_plate = Label3D.new()
	_plate.billboard = BaseMaterial3D.BILLBOARD_ENABLED
	_plate.position = Vector3(0, 1.8, 0)
	_plate.font_size = 40
	_plate.pixel_size = 0.005
	_plate.outline_size = 12
	_plate.text = display_name
	_plate.modulate = Color(1.0, 0.85, 0.4)
	add_child(_plate)


const EFFECT_COLORS := {
	1: Color(1.0, 0.8, 0.25),  # vigor
	2: Color(0.7, 0.4, 1.0),  # weakened
	4: Color(1.0, 0.2, 0.15),  # enraged
	8: Color(0.35, 0.35, 0.45),  # ill omen
	64: Color(1.0, 0.95, 0.3),  # stunned
	32: Color(1.0, 0.45, 0.1),  # burning
	16: Color(0.55, 0.8, 1.0),  # ward
	128: Color(0.3, 0.85, 0.8),  # slowed
	256: Color(0.4, 1.0, 0.5),  # regen
}
const EFFECT_NAMES := {
	1: "Vigor", 2: "Weakened", 4: "Enraged", 8: "Ill Omen", 16: "Ward",
	32: "Burning", 64: "Stunned", 128: "Slowed", 256: "Regen",
}

var effects := 0
var _aura: MeshInstance3D


## Shows active Fate effects (EffectFlag bits) as a colored aura at the feet.
func set_effects(flags: int) -> void:
	if flags == effects or is_object():
		return
	_effects_gained(flags & ~effects)
	effects = flags
	if flags == 0:
		if _aura != null:
			_aura.queue_free()
			_aura = null
		return
	if _aura == null:
		_aura = MeshInstance3D.new()
		var cyl := CylinderMesh.new()
		cyl.top_radius = 0.75
		cyl.bottom_radius = 0.75
		cyl.height = 2.1
		cyl.cap_top = false
		cyl.cap_bottom = false
		_aura.mesh = cyl
		_aura.position = Vector3(0, 1.05, 0)
		add_child(_aura)
	var color := Color.WHITE
	for bit in EFFECT_COLORS:
		if flags & bit:
			color = EFFECT_COLORS[bit]
			break
	var mat := StandardMaterial3D.new()
	mat.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	mat.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
	mat.cull_mode = BaseMaterial3D.CULL_DISABLED
	mat.albedo_color = Color(color, 0.22)
	_aura.material_override = mat


## A short burst when an effect lands (the aura then shows it lasting).
func _effects_gained(bits: int) -> void:
	var world := get_parent()
	if world == null or bits == 0:
		return
	var VFX = load("res://game/vfx.gd")
	if bits & 2:  # weakened
		VFX.mark(world, self)
	if bits & 64:  # stunned
		VFX.burst(world, global_position + Vector3(0, 2.2, 0), Color(1.0, 0.95, 0.3), 14, 1.2, 0.06, 0.0)
	if bits & 32:  # burning
		VFX.burst(world, global_position + Vector3(0, 1.0, 0), Color(1.0, 0.5, 0.1), 20, 2.0, 0.06, 2.0)
	if bits & 16:  # ward
		VFX.shockwave(world, global_position, 1.6, "ward")
	if bits & 1 or bits & 4:  # vigor / enraged
		VFX.flash(world, global_position + Vector3(0, 1.2, 0), Color(1.0, 0.6, 0.2), 3.0)


func effect_names() -> PackedStringArray:
	var out := PackedStringArray()
	for bit in EFFECT_NAMES:
		if effects & bit:
			out.append(EFFECT_NAMES[bit])
	return out


func is_alive() -> bool:
	return hp > 0


func chest() -> Vector3:
	return global_position + Vector3(0, 1.2, 0)


func set_health(new_hp: int, new_max: int) -> void:
	var was_alive := hp > 0
	hp = new_hp
	max_hp = maxi(new_max, 1)
	if was_alive and hp <= 0:
		_oneshot = true
		_play(ANIM_DEATH, 0.1)
	elif not was_alive and hp > 0:
		_oneshot = false
		_play(ANIM_MOVE if _moving else ANIM_IDLE)
	_update_plate()


## Plays a one-shot clip (attack, hit), then returns to idle/run.
func play_action(anim: String) -> void:
	if hp <= 0 or _player == null or not _player.has_animation(anim):
		return
	_oneshot = true
	_play(anim, 0.08)
	_player.seek(0.0, true)


func play_hit() -> void:
	# Do not interrupt an attack with a flinch.
	if not _oneshot:
		play_action(ANIM_HIT)


## Floating combat text above the head.
func show_number(text: String, color: Color) -> void:
	var label := Label3D.new()
	label.text = text
	label.modulate = color
	label.billboard = BaseMaterial3D.BILLBOARD_ENABLED
	label.font_size = 64
	label.pixel_size = 0.005
	label.outline_size = 14
	label.no_depth_test = true
	label.position = Vector3(randf_range(-0.3, 0.3), 2.3, 0)
	add_child(label)
	var tw := label.create_tween().set_parallel(true)
	tw.tween_property(label, "position:y", 3.2, 0.9).set_ease(Tween.EASE_OUT)
	tw.tween_property(label, "modulate:a", 0.0, 0.9).set_delay(0.3)
	tw.chain().tween_callback(label.queue_free)


## Adds a server state for interpolation (remote entities).
func push_state(tick: float, pos: Vector3, yaw: float, moving: bool) -> void:
	if not _buffer.is_empty() and tick <= _buffer[-1]["tick"]:
		return
	_buffer.append({"tick": tick, "pos": pos, "yaw": yaw, "moving": moving})
	if _buffer.size() > MAX_BUFFER:
		_buffer.pop_front()
	if _buffer.size() == 1:
		_apply(pos, yaw, moving)


## Positions the entity at render_tick by interpolating buffered states.
func interpolate(render_tick: float) -> void:
	if _buffer.is_empty():
		return
	while _buffer.size() >= 2 and _buffer[1]["tick"] <= render_tick:
		_buffer.pop_front()
	var a: Dictionary = _buffer[0]
	if _buffer.size() == 1 or render_tick <= a["tick"]:
		_apply(a["pos"], a["yaw"], a["moving"])
		return
	var b: Dictionary = _buffer[1]
	var t := clampf((render_tick - a["tick"]) / (b["tick"] - a["tick"]), 0.0, 1.0)
	_apply((a["pos"] as Vector3).lerp(b["pos"], t), lerp_angle(a["yaw"], b["yaw"], t), b["moving"])


## Local player: the world passes the predicted transform every frame.
func set_predicted(pos: Vector3, yaw: float, moving: bool) -> void:
	_apply(pos, yaw, moving)


func _apply(pos: Vector3, yaw: float, moving: bool) -> void:
	position = pos
	rotation.y = yaw
	if moving != _moving:
		_moving = moving
		# Running cancels a flinch so feet never slide; attacks, blocks and
		# rolls keep their animation.
		if hp > 0 and action == 0 and (not _oneshot or moving):
			_oneshot = false
			_play(ANIM_MOVE if moving else ANIM_IDLE)


## Plays the animation for an action state. key changes for each new
## instance of a repeated action (a chained swing), so it replays.
func set_action(state: int, key: int) -> void:
	if state == action and key == _action_key:
		return
	var prev := action
	action = state
	_action_key = key
	if hp <= 0 or _player == null:
		return
	var w := ContentDB.weapon_for(class_key)
	match state:
		1:  # swing
			play_action(w.get("anim", "Punch_Cross"))
		2:  # flourish
			play_action(w.get("flourish_anim", "Sword_Heavy_Combo"))
		3:  # charging
			_hold({"melee": "Sword_Idle", "ranged": "Pistol_Aim_Neutral"}.get(w.get("kind", "melee"), "Sword_Idle")
					if class_key != "mystic" else "Spell_Simple_Idle")
		4:  # block
			_hold("Sword_Block")
		5:  # roll
			play_action("Roll")
		6:  # NPC wind-up
			play_action(attack_anim if not attack_anim.is_empty() else "Punch_Jab")
		7:  # NPC slam wind-up
			play_action("Sword_Heavy_Combo")
		_:
			if prev in [3, 4]:
				_oneshot = false
				_play(ANIM_MOVE if _moving else ANIM_IDLE)


## Holds a pose (block, charge) until the action changes.
func _hold(anim: String) -> void:
	if _player.has_animation(anim):
		_oneshot = true
		_play(anim, 0.1)


func _on_animation_finished(anim: StringName) -> void:
	if anim == ANIM_DEATH or hp <= 0 or action in [3, 4]:
		return
	_oneshot = false
	_play(ANIM_MOVE if _moving else ANIM_IDLE)


func _play(anim: String, blend := ANIM_BLEND) -> void:
	if _player != null and _player.has_animation(anim):
		_player.play(anim, blend)


func _update_plate() -> void:
	if _plate == null or is_object():
		return
	if is_npc():
		_plate.text = "%s  Lv %d" % [display_name, level]
		_plate.modulate = Color(1.0, 0.45, 0.4) if hp > 0 else Color(0.6, 0.6, 0.6)
	else:
		_plate.text = display_name
		_plate.modulate = Color(1.0, 0.92, 0.6) if is_local else Color.WHITE
