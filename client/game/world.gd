## The in-zone scene: environment, camera, local prediction, replicated
## entities and tab-target combat.
##
## Local player: every server tick (50 ms) the current input is sent as a
## MoveInput and applied locally right away (prediction). Each Snapshot carries
## the server position and the last input it applied; we reset to that
## position and replay the inputs it has not applied yet (reconciliation).
##
## Combat: the player selects a target (click or Tab) and uses skills (1, 2).
## Cooldowns and range are checked locally for responsiveness, but the server
## decides; rejections come back through Net.skill_rejected.
extends Node3D

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const Character := preload("res://game/character.gd")
const Hud := preload("res://game/hud.gd")
const INTERP_TICKS := 2.0  # remote entities render this far in the past
const SNAP_DISTANCE := 3.0  # corrections larger than this teleport instead of blending
const TAB_RANGE := 30.0
const NPC_ATTACK_ANIM := "Punch_Jab"
const REJECT_TEXT := {
	"cooldown": "Not ready yet",
	"out_of_range": "Out of range",
	"invalid_target": "Invalid target",
	"dead": "You are dead",
	"unknown_skill": "Unknown skill",
}

## Test options.
var autowalk := false
var autofight := false

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

var _skills := {}  # id -> PB.SkillInfo
var _skill_order: Array[int] = []
var _ready_at := {}  # skill id -> Time.get_ticks_msec() when usable again
var _status: PB.PlayerStatus
var _target_id := 0
var _ring: MeshInstance3D

var _camera: Camera3D
var _cam_yaw := 0.0
var _cam_pitch := 0.45
var _cam_distance := 7.0
var _click_start := Vector2.ZERO
var _hud: Hud


func start(welcome: PB.Welcome) -> void:
	my_id = welcome.entity_id
	zone_id = welcome.zone_id
	tick_rate = maxi(welcome.tick_rate, 1)
	move_speed = welcome.move_speed
	half_extent = welcome.half_extent
	_pred_pos = _vec(welcome.spawn)
	_prev_pred_pos = _pred_pos
	_client_tick = welcome.server_tick
	for s in welcome.skills:
		_skills[s.id] = s
		_skill_order.append(s.id)
	_build_environment()
	_build_camera()
	_hud = Hud.new()
	add_child(_hud)
	_hud.set_skills(welcome.skills)
	_hud.skill_pressed.connect(use_skill)
	_build_ring()
	if welcome.status != null:
		_on_status(welcome.status)
	_hud.set_inventory(welcome.inventory)
	Net.snapshot_received.connect(_on_snapshot)
	Net.skill_rejected.connect(_on_skill_rejected)
	Net.fate_received.connect(_on_fate)


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

	if autofight:
		_autofight()
	_update_target_visuals()
	_update_camera(local.position if local != null else _pred_pos)
	_update_hud()


func _is_dead() -> bool:
	return _status != null and _status.hp == 0


# --- Input and prediction ----------------------------------------------------

func _read_input() -> Vector2:
	if _is_dead():
		return Vector2.ZERO
	if autofight:
		var target: Character = _entities.get(_target_id)
		if target != null and target.is_alive():
			var to := Vector2(target.global_position.x - _pred_pos.x, target.global_position.z - _pred_pos.z)
			return to.normalized() if to.length() > 1.8 else Vector2.ZERO
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


# --- Targeting and skills ----------------------------------------------------

func _unhandled_input(event: InputEvent) -> void:
	if event is InputEventMouseMotion and (event.button_mask & MOUSE_BUTTON_MASK_RIGHT):
		_cam_yaw -= event.relative.x * 0.006
		_cam_pitch = clampf(_cam_pitch + event.relative.y * 0.004, 0.05, 1.3)
	elif event is InputEventMouseButton:
		if event.button_index == MOUSE_BUTTON_WHEEL_UP and event.pressed:
			_cam_distance = maxf(_cam_distance - 0.6, 2.5)
		elif event.button_index == MOUSE_BUTTON_WHEEL_DOWN and event.pressed:
			_cam_distance = minf(_cam_distance + 0.6, 25.0)
		elif event.button_index == MOUSE_BUTTON_LEFT:
			if event.pressed:
				_click_start = event.position
			elif event.position.distance_to(_click_start) < 6.0:
				_pick_at(event.position)
	elif event is InputEventKey and event.pressed and not event.echo:
		match event.physical_keycode:
			KEY_TAB:
				_cycle_target()
				get_viewport().set_input_as_handled()
			KEY_I:
				_hud.toggle_inventory()
			KEY_1, KEY_2, KEY_3, KEY_4:
				var i: int = event.physical_keycode - KEY_1
				if i < _skill_order.size():
					use_skill(_skill_order[i])


func _pick_at(screen_pos: Vector2) -> void:
	var from := _camera.project_ray_origin(screen_pos)
	var query := PhysicsRayQueryParameters3D.create(from, from + _camera.project_ray_normal(screen_pos) * 200.0)
	query.collision_mask = Character.PICK_LAYER
	var hit := get_world_3d().direct_space_state.intersect_ray(query)
	if hit.is_empty():
		_set_target(0)
	elif hit["collider"].has_meta("entity_id"):
		var id: int = hit["collider"].get_meta("entity_id")
		_set_target(0 if id == my_id else id)


## Tab: the nearest living NPC, or the next one by distance if already targeting.
func _cycle_target() -> void:
	var candidates: Array = []
	for id in _entities:
		var c: Character = _entities[id]
		if c.is_npc() and c.is_alive():
			var d := c.global_position.distance_to(_pred_pos)
			if d <= TAB_RANGE:
				candidates.append([d, id])
	if candidates.is_empty():
		_hud.show_message("No enemies nearby", Color(0.8, 0.8, 0.8))
		return
	candidates.sort_custom(func(a, b): return a[0] < b[0])
	var next: int = candidates[0][1]
	for i in candidates.size():
		if candidates[i][1] == _target_id:
			next = candidates[(i + 1) % candidates.size()][1]
			break
	_set_target(next)


func _set_target(id: int) -> void:
	_target_id = id if _entities.has(id) else 0
	_hud.set_target(_entities.get(_target_id))


func use_skill(skill_id: int) -> void:
	var s: PB.SkillInfo = _skills.get(skill_id)
	if s == null or _is_dead():
		return
	var target: Character = _entities.get(_target_id)
	if target == null or not target.is_npc():
		_hud.show_message("Select an enemy first (click or Tab)", Color(1, 0.85, 0.5))
		return
	if not target.is_alive():
		_hud.show_message("Target is dead", Color(0.8, 0.8, 0.8))
		return
	if Time.get_ticks_msec() < _ready_at.get(skill_id, 0):
		_hud.show_message("Not ready yet", Color(0.8, 0.8, 0.8))
		return
	if target.global_position.distance_to(_pred_pos) > s.range + 0.5:
		_hud.show_message("Out of range", Color(1, 0.85, 0.5))
		return
	_ready_at[skill_id] = Time.get_ticks_msec() + s.cooldown_ms
	Net.send_skill(skill_id, _target_id)


func _on_skill_rejected(skill_id: int, reason: String) -> void:
	_ready_at.erase(skill_id)  # let the player retry right away
	_hud.show_message(REJECT_TEXT.get(reason, reason), Color(1, 0.6, 0.5))


## Test option: target the nearest enemy, walk to it and use skills.
func _autofight() -> void:
	var target: Character = _entities.get(_target_id)
	if target == null or not target.is_alive():
		_cycle_target()
		return
	var now := Time.get_ticks_msec()
	for id in _skill_order:
		var s: PB.SkillInfo = _skills[id]
		if now >= _ready_at.get(id, 0) and target.global_position.distance_to(_pred_pos) <= s.range:
			use_skill(id)


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
			if id == _target_id:
				_set_target(0)

	for spawn in snap.spawns:
		if _entities.has(spawn.id):
			continue
		var c := Character.new()
		add_child(c)
		c.setup(spawn, spawn.id == my_id)
		_entities[spawn.id] = c
		_apply_state(snap, spawn.state)

	for state in snap.entities:
		_apply_state(snap, state)

	for ev in snap.events:
		_on_combat_event(ev)

	if snap.status != null:
		_on_status(snap.status)


func _apply_state(snap: PB.Snapshot, state: PB.EntityState) -> void:
	if state == null or not _entities.has(state.id):
		return
	var c: Character = _entities[state.id]
	c.set_health(state.hp, state.max_hp)
	c.set_effects(state.effects)
	if state.id == my_id:
		_reconcile(_vec(state.pos), snap.ack_input_seq)
	else:
		c.push_state(snap.tick, _vec(state.pos), state.yaw, state.moving)


func _on_combat_event(ev: PB.CombatEvent) -> void:
	var src: Character = _entities.get(ev.source_id)
	var dst: Character = _entities.get(ev.target_id)
	match ev.kind:
		PB.CombatEventKind.COMBAT_EVENT_KIND_DAMAGE:
			var skill: PB.SkillInfo = _skills.get(ev.skill_id)
			if src != null:
				src.play_action(skill.anim if skill != null else NPC_ATTACK_ANIM)
			if dst == null:
				return
			var color := Color(1.0, 0.95, 0.85)
			if ev.target_id == my_id:
				color = Color(1.0, 0.35, 0.3)
			elif ev.source_id != my_id:
				color = Color(0.75, 0.75, 0.75)
			var amount := str(ev.amount)
			if ev.critical:
				amount = "%d!" % ev.amount
				color = Color(1.0, 0.8, 0.2)
			if skill != null and skill.projectile and src != null:
				_fire_projectile(src, dst, func(): _land_hit(dst, amount, color))
			else:
				_land_hit(dst, amount, color)
		PB.CombatEventKind.COMBAT_EVENT_KIND_MISS:
			if src != null:
				var miss_skill: PB.SkillInfo = _skills.get(ev.skill_id)
				src.play_action(miss_skill.anim if miss_skill != null else NPC_ATTACK_ANIM)
			if dst != null:
				dst.show_number("Miss", Color(0.7, 0.7, 0.7))
		PB.CombatEventKind.COMBAT_EVENT_KIND_DEATH:
			if dst != null and ev.source_id == my_id and dst.is_npc():
				_hud.show_message("%s defeated" % dst.display_name, Color(0.9, 0.85, 0.5))


func _land_hit(dst: Character, amount: String, color: Color) -> void:
	if not is_instance_valid(dst):
		return
	dst.show_number(amount, color)
	if dst.is_alive():
		dst.play_hit()


func _fire_projectile(src: Character, dst: Character, on_hit: Callable) -> void:
	var orb := MeshInstance3D.new()
	var sphere := SphereMesh.new()
	sphere.radius = 0.14
	sphere.height = 0.28
	orb.mesh = sphere
	var mat := StandardMaterial3D.new()
	mat.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	mat.albedo_color = Color(0.5, 0.95, 1.0)
	mat.emission_enabled = true
	mat.emission = Color(0.4, 0.9, 1.0)
	orb.material_override = mat
	add_child(orb)
	orb.global_position = src.chest()
	var end := dst.chest()
	var tw := orb.create_tween()
	tw.tween_property(orb, "global_position", end, clampf(src.chest().distance_to(end) / 30.0, 0.1, 0.6))
	tw.tween_callback(on_hit)
	tw.tween_callback(orb.queue_free)


func _on_fate(ev: PB.FateEvent) -> void:
	var effect := ""
	match ev.action:
		"treasure", "spoils", "cache_found":
			effect = "Obtained: %s" % ev.item_name
		"hidden_cache":
			effect = "A hidden cache appeared nearby. Find the glowing light!"
		"blessing":
			effect = "Healed %d" % ev.amount
		"insight":
			effect = "+%d exp" % ev.amount
		"critical":
			effect = "Critical strike!"
		"weakness":
			effect = "Your target is weakened for %ds" % (ev.duration_ms / 1000)
		"vigor_aura":
			effect = "Aura of Vigor for %ds" % (ev.duration_ms / 1000)
		"ambush":
			effect = "Ambush! %d enemies" % ev.amount
		"mishap":
			effect = "-%d HP" % ev.amount
		"fumble":
			effect = "Your attack missed"
		"enrage":
			effect = "Your target is enraged for %ds" % (ev.duration_ms / 1000)
		"ill_omen":
			effect = "Ill Omen for %ds" % (ev.duration_ms / 1000)
	_hud.show_fate(ev, effect)
	if not ev.inventory.is_empty():
		_hud.set_inventory(ev.inventory)


func _on_status(s: PB.PlayerStatus) -> void:
	if _status != null:
		if s.level > _status.level:
			_hud.show_message("Level up! You are now level %d" % s.level, Color(1.0, 0.85, 0.3))
		elif s.exp > _status.exp and s.level == _status.level:
			_hud.show_message("+%d exp" % (s.exp - _status.exp), Color(0.9, 0.85, 0.5))
		if _status.hp > 0 and s.hp == 0:
			_set_target(0)
	_status = s
	_hud.set_status(s)


static func _vec(v: PB.Vec3) -> Vector3:
	return Vector3.ZERO if v == null else Vector3(v.x, v.y, v.z)


# --- Camera, target ring, HUD ------------------------------------------------

func set_camera_yaw(yaw: float) -> void:
	_cam_yaw = yaw


func _update_camera(target: Vector3) -> void:
	var offset := Vector3(sin(_cam_yaw) * cos(_cam_pitch), sin(_cam_pitch), cos(_cam_yaw) * cos(_cam_pitch))
	var focus := target + Vector3(0, 1.4, 0)
	_camera.position = focus + offset * _cam_distance
	_camera.look_at(focus)


func _update_target_visuals() -> void:
	var target: Character = _entities.get(_target_id)
	_ring.visible = target != null
	if target != null:
		_ring.global_position = target.global_position + Vector3(0, 0.04, 0)
		_hud.set_target(target)
	var now := Time.get_ticks_msec()
	var remaining := {}
	for id in _ready_at:
		remaining[id] = maxf(0.0, (_ready_at[id] - now) / 1000.0)
	_hud.update_cooldowns(remaining)


func _update_hud() -> void:
	_hud.debug_label.text = "%s  ·  entity %d  ·  visible %d  ·  rtt %d ms\nWASD move · click/Tab target · 1-2 skills · I inventory · right-drag camera · Esc logout" % [
		zone_id, my_id, _entities.size(), Net.rtt_ms]


func _build_ring() -> void:
	_ring = MeshInstance3D.new()
	var torus := TorusMesh.new()
	torus.inner_radius = 0.55
	torus.outer_radius = 0.68
	_ring.mesh = torus
	_ring.scale = Vector3(1, 0.05, 1)
	var mat := StandardMaterial3D.new()
	mat.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	mat.albedo_color = Color(1.0, 0.35, 0.25)
	_ring.material_override = mat
	_ring.visible = false
	add_child(_ring)


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
