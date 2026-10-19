## The in-zone scene: environment, camera, local prediction and replicated
## entities.
##
## Local player: every server tick (50 ms) the current input is sent as a
## MoveInput and applied locally right away (prediction). Each Snapshot carries
## the server position and the last input it applied; we reset to that
## position and replay the inputs it has not applied yet (reconciliation).
extends Node3D

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const Character := preload("res://game/character.gd")
const INTERP_TICKS := 2.0  # remote entities render this far in the past
const SNAP_DISTANCE := 3.0  # corrections larger than this teleport instead of blending

## Test option: walk in a circle without keyboard input.
var autowalk := false

var my_id := 0
var zone_id := ""
var tick_rate := 20
var move_speed := 5.0
var half_extent := 60.0

var _entities := {}  # id -> Character
var _client_tick := 0.0
var _clock_synced := false

var _pending: Array[Dictionary] = []  # unacknowledged inputs: {seq, dir: Vector2}
var _seq := 0
var _acc := 0.0
var _was_moving := false
var _pred_pos := Vector3.ZERO
var _prev_pred_pos := Vector3.ZERO
var _pred_yaw := 0.0
var _pred_moving := false

var _camera: Camera3D
var _cam_yaw := 0.0
var _cam_pitch := 0.45
var _cam_distance := 7.0
var _hud: Label


func start(welcome: PB.Welcome) -> void:
	my_id = welcome.entity_id
	zone_id = welcome.zone_id
	tick_rate = maxi(welcome.tick_rate, 1)
	move_speed = welcome.move_speed
	half_extent = welcome.half_extent
	_pred_pos = _vec(welcome.spawn)
	_prev_pred_pos = _pred_pos
	_client_tick = welcome.server_tick
	_build_environment()
	_build_camera()
	_build_hud()
	Net.snapshot_received.connect(_on_snapshot)


func _process(delta: float) -> void:
	var tick_dt := 1.0 / tick_rate
	_client_tick += delta * tick_rate
	_acc += delta
	var steps := 0
	while _acc >= tick_dt:
		_acc -= tick_dt
		steps += 1
		if steps <= 3:  # after a long hitch, skip rather than flood the server
			_step_input()
	if steps > 3:
		_acc = 0.0

	var local: Character = _entities.get(my_id)
	if local != null:
		var t := clampf(_acc / tick_dt, 0.0, 1.0)
		local.set_predicted(_prev_pred_pos.lerp(_pred_pos, t), _pred_yaw, _pred_moving)

	var render_tick := _client_tick - INTERP_TICKS
	for id in _entities:
		if id != my_id:
			_entities[id].interpolate(render_tick)

	_update_camera(local.position if local != null else _pred_pos)
	_update_hud()


# --- Input and prediction ----------------------------------------------------

func _read_input() -> Vector2:
	if autowalk:
		var t := Time.get_ticks_msec() / 1000.0 * 0.6
		return Vector2(cos(t), sin(t))
	var fwd := 0.0
	var right := 0.0
	if not get_window().has_focus():
		return Vector2.ZERO
	if Input.is_physical_key_pressed(KEY_W) or Input.is_physical_key_pressed(KEY_UP):
		fwd += 1.0
	if Input.is_physical_key_pressed(KEY_S) or Input.is_physical_key_pressed(KEY_DOWN):
		fwd -= 1.0
	if Input.is_physical_key_pressed(KEY_D) or Input.is_physical_key_pressed(KEY_RIGHT):
		right += 1.0
	if Input.is_physical_key_pressed(KEY_A) or Input.is_physical_key_pressed(KEY_LEFT):
		right -= 1.0
	# Camera-relative: forward points from the camera toward the player.
	var f := Vector2(-sin(_cam_yaw), -cos(_cam_yaw))
	var r := Vector2(cos(_cam_yaw), -sin(_cam_yaw))
	var dir := f * fwd + r * right
	return dir.normalized() if dir.length() > 1.0 else dir


func _step_input() -> void:
	var dir := _read_input()
	var moving := dir.length_squared() > 0.0001
	if moving:
		_pred_yaw = atan2(dir.x, dir.y)
	_prev_pred_pos = _pred_pos
	# Send while moving, plus one final stop so the server sees it at once.
	if moving or _was_moving:
		_seq += 1
		Net.send_move(_seq, dir.x, dir.y, _pred_yaw)
		_pending.append({"seq": _seq, "dir": dir})
		_pred_pos = _simulate(_pred_pos, dir)
	_was_moving = moving
	_pred_moving = moving


## Must match zone.applyInputs on the server.
func _simulate(pos: Vector3, dir: Vector2) -> Vector3:
	if dir.length() > 1.0:
		dir = dir.normalized()
	var step := move_speed / tick_rate
	pos.x = clampf(pos.x + dir.x * step, -half_extent, half_extent)
	pos.z = clampf(pos.z + dir.y * step, -half_extent, half_extent)
	return pos


func _reconcile(server_pos: Vector3, ack: int) -> void:
	while not _pending.is_empty() and _pending[0]["seq"] <= ack:
		_pending.pop_front()
	var corrected := server_pos
	for input in _pending:
		corrected = _simulate(corrected, input["dir"])
	var error := corrected - _pred_pos
	if error.length() > SNAP_DISTANCE:
		_prev_pred_pos = corrected
	else:
		_prev_pred_pos += error
	_pred_pos = corrected


# --- Replication -------------------------------------------------------------

func _on_snapshot(snap: PB.Snapshot) -> void:
	var diff := snap.tick - _client_tick
	if not _clock_synced or absf(diff) > 10.0:
		_client_tick = snap.tick
		_clock_synced = true
	else:
		_client_tick += diff * 0.1

	for id in snap.despawns:
		if _entities.has(id):
			_entities[id].queue_free()
			_entities.erase(id)

	for spawn in snap.spawns:
		if _entities.has(spawn.id):
			continue
		var c := Character.new()
		add_child(c)
		c.setup(spawn.id, spawn.name, spawn.model, spawn.id == my_id)
		_entities[spawn.id] = c
		_apply_state(snap, spawn.state)

	for state in snap.entities:
		_apply_state(snap, state)


func _apply_state(snap: PB.Snapshot, state: PB.EntityState) -> void:
	if state == null or not _entities.has(state.id):
		return
	if state.id == my_id:
		_reconcile(_vec(state.pos), snap.ack_input_seq)
	else:
		_entities[state.id].push_state(snap.tick, _vec(state.pos), state.yaw, state.moving)


static func _vec(v: PB.Vec3) -> Vector3:
	return Vector3.ZERO if v == null else Vector3(v.x, v.y, v.z)


# --- Camera ------------------------------------------------------------------

func _unhandled_input(event: InputEvent) -> void:
	if event is InputEventMouseMotion and (event.button_mask & MOUSE_BUTTON_MASK_RIGHT):
		_cam_yaw -= event.relative.x * 0.006
		_cam_pitch = clampf(_cam_pitch + event.relative.y * 0.004, 0.05, 1.3)
	elif event is InputEventMouseButton and event.pressed:
		if event.button_index == MOUSE_BUTTON_WHEEL_UP:
			_cam_distance = maxf(_cam_distance - 0.6, 2.5)
		elif event.button_index == MOUSE_BUTTON_WHEEL_DOWN:
			_cam_distance = minf(_cam_distance + 0.6, 25.0)


func set_camera_yaw(yaw: float) -> void:
	_cam_yaw = yaw


func _update_camera(target: Vector3) -> void:
	var offset := Vector3(sin(_cam_yaw) * cos(_cam_pitch), sin(_cam_pitch), cos(_cam_yaw) * cos(_cam_pitch))
	var focus := target + Vector3(0, 1.4, 0)
	_camera.position = focus + offset * _cam_distance
	_camera.look_at(focus)


# --- Scene construction ------------------------------------------------------

func _build_camera() -> void:
	_camera = Camera3D.new()
	_camera.fov = 60
	add_child(_camera)
	_camera.make_current()


func _build_environment() -> void:
	var env := Environment.new()
	var sky_mat := ProceduralSkyMaterial.new()
	sky_mat.sky_top_color = Color(0.38, 0.55, 0.72)
	sky_mat.sky_horizon_color = Color(0.78, 0.80, 0.74)
	sky_mat.ground_horizon_color = Color(0.55, 0.58, 0.50)
	var sky := Sky.new()
	sky.sky_material = sky_mat
	env.background_mode = Environment.BG_SKY
	env.sky = sky
	env.ambient_light_source = Environment.AMBIENT_SOURCE_SKY
	env.tonemap_mode = Environment.TONE_MAPPER_FILMIC
	env.fog_enabled = true
	env.fog_light_color = Color(0.75, 0.80, 0.78)
	env.fog_density = 0.006
	var we := WorldEnvironment.new()
	we.environment = env
	add_child(we)

	var sun := DirectionalLight3D.new()
	sun.rotation_degrees = Vector3(-50, -35, 0)
	sun.light_energy = 1.2
	sun.shadow_enabled = true
	add_child(sun)

	# Ground with a 2 m grid so movement reads clearly.
	var ground := MeshInstance3D.new()
	var plane := PlaneMesh.new()
	plane.size = Vector2(half_extent * 2.0 + 20.0, half_extent * 2.0 + 20.0)
	ground.mesh = plane
	var shader := Shader.new()
	shader.code = """
shader_type spatial;
uniform vec3 base_color : source_color = vec3(0.33, 0.43, 0.30);
uniform vec3 line_color : source_color = vec3(0.24, 0.32, 0.22);
uniform vec3 edge_color : source_color = vec3(0.55, 0.47, 0.32);
uniform float half_extent = 60.0;
varying vec3 world_pos;
void vertex() { world_pos = (MODEL_MATRIX * vec4(VERTEX, 1.0)).xyz; }
void fragment() {
	vec2 g = abs(fract(world_pos.xz * 0.5 + 0.5) - 0.5);
	float line = 1.0 - step(0.02, min(g.x, g.y));
	vec3 c = mix(base_color, line_color, line);
	float outside = step(half_extent, max(abs(world_pos.x), abs(world_pos.z)));
	ALBEDO = mix(c, edge_color, outside);
	ROUGHNESS = 1.0;
}
"""
	var mat := ShaderMaterial.new()
	mat.shader = shader
	mat.set_shader_parameter("half_extent", half_extent)
	ground.material_override = mat
	add_child(ground)

	# A few stone pillars as landmarks (visual only; no server collision yet).
	var stone := StandardMaterial3D.new()
	stone.albedo_color = Color(0.62, 0.60, 0.55)
	stone.roughness = 0.9
	for p in [Vector3(8, 0, -6), Vector3(-10, 0, -12), Vector3(14, 0, 10), Vector3(-6, 0, 9), Vector3(0, 0, -20)]:
		var pillar := MeshInstance3D.new()
		var cyl := CylinderMesh.new()
		cyl.top_radius = 0.6
		cyl.bottom_radius = 0.8
		cyl.height = 4.0
		pillar.mesh = cyl
		pillar.material_override = stone
		pillar.position = p + Vector3(0, 2, 0)
		add_child(pillar)


func _build_hud() -> void:
	var layer := CanvasLayer.new()
	add_child(layer)
	_hud = Label.new()
	_hud.position = Vector2(12, 10)
	_hud.add_theme_color_override("font_outline_color", Color.BLACK)
	_hud.add_theme_constant_override("outline_size", 6)
	layer.add_child(_hud)


func _update_hud() -> void:
	_hud.text = "%s  ·  entity %d  ·  players %d  ·  rtt %d ms  ·  pending %d\npos (%.1f, %.1f)\nWASD move · right-drag camera · wheel zoom · Esc disconnect" % [
		zone_id, my_id, _entities.size(), Net.rtt_ms, _pending.size(), _pred_pos.x, _pred_pos.z]
