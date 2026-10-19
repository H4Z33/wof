## A replicated entity: model, animation, nameplate and interpolation.
##
## Remote entities render ~100 ms in the past, interpolating between buffered
## server states. The local player is positioned by the world's prediction
## code through set_predicted().
extends Node3D

const Models := preload("res://game/models.gd")
# Godot's glTF importer strips the "_Loop" suffix and marks those clips as looping.
const ANIM_IDLE := "Idle"
const ANIM_MOVE := "Jog_Fwd"
const ANIM_BLEND := 0.2
const MAX_BUFFER := 40

var entity_id := 0
var is_local := false

var _player: AnimationPlayer
var _moving := false
# Each entry: {tick: float, pos: Vector3, yaw: float, moving: bool}
var _buffer: Array[Dictionary] = []


func setup(id: int, display_name: String, model: String, local: bool) -> void:
	entity_id = id
	is_local = local
	name = "Entity_%d" % id
	var parts := Models.instantiate(model)
	add_child(parts[0])
	_player = parts[1]
	_play(ANIM_IDLE)

	var plate := Label3D.new()
	plate.text = display_name
	plate.billboard = BaseMaterial3D.BILLBOARD_ENABLED
	plate.position = Vector3(0, 2.15, 0)
	plate.font_size = 48
	plate.pixel_size = 0.005
	plate.outline_size = 12
	plate.modulate = Color(1.0, 0.92, 0.6) if local else Color.WHITE
	plate.no_depth_test = true
	add_child(plate)


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
		_play(ANIM_MOVE if moving else ANIM_IDLE)


func _play(anim: String) -> void:
	if _player != null and _player.has_animation(anim):
		_player.play(anim, ANIM_BLEND)
