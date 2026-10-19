## A replicated entity: model, animation, nameplate, health and interpolation.
##
## Remote entities render ~100 ms in the past, interpolating between buffered
## server states. The local player is positioned by the world's prediction
## code through set_predicted().
extends Node3D

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const Models := preload("res://game/models.gd")
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
	level = maxi(spawn.level, 1)
	name = "Entity_%d" % entity_id
	var parts := Models.instantiate(spawn.model)
	add_child(parts[0])
	_player = parts[1]
	_player.animation_finished.connect(_on_animation_finished)

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
		# Running cancels an attack/flinch so feet never slide.
		if hp > 0 and (not _oneshot or moving):
			_oneshot = false
			_play(ANIM_MOVE if moving else ANIM_IDLE)


func _on_animation_finished(anim: StringName) -> void:
	if anim == ANIM_DEATH or hp <= 0:
		return
	_oneshot = false
	_play(ANIM_MOVE if _moving else ANIM_IDLE)


func _play(anim: String, blend := ANIM_BLEND) -> void:
	if _player != null and _player.has_animation(anim):
		_player.play(anim, blend)


func _update_plate() -> void:
	if _plate == null:
		return
	if is_npc():
		_plate.text = "%s  Lv %d" % [display_name, level]
		_plate.modulate = Color(1.0, 0.45, 0.4) if hp > 0 else Color(0.6, 0.6, 0.6)
	else:
		_plate.text = display_name
		_plate.modulate = Color(1.0, 0.92, 0.6) if is_local else Color.WHITE
