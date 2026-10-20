## The in-zone scene: environment, camera, local prediction, replicated
## entities and action combat.
##
## Local player: every server tick (50 ms) the current input is sent as a
## MoveInput and applied locally right away (prediction). Each Snapshot carries
## the server position and the last input it applied; we reset to that
## position and replay the inputs it has not applied yet (reconciliation).
##
## Combat (Fable-style): left click attacks (hold to charge a flourish), right
## click blocks, Space rolls, F locks on. The action state machine runs here
## and on the server from the same inputs. Skills (1-4) go at the locked
## target or the nearest enemy in front; the server decides, and rejections
## come back through Net.skill_rejected.
extends Node3D

const PB := preload("res://gen/convergence/v1/world.pb.gd")
const Character := preload("res://game/character.gd")
const Hud := preload("res://game/hud.gd")
const Terrain := preload("res://game/terrain.gd")
const VFX := preload("res://game/vfx.gd")
const ContentDB := preload("res://game/content_db.gd")
const INTERP_TICKS := 2.0  # remote entities render this far in the past
const SNAP_DISTANCE := 3.0  # corrections larger than this teleport instead of blending
const TAB_RANGE := 40.0
const NPC_ATTACK_ANIM := "Punch_Jab"
const REJECT_TEXT := {
	"cooldown": "Not ready yet",
	"out_of_range": "Out of range",
	"invalid_target": "Invalid target",
	"dead": "You are dead",
	"unknown_skill": "Unknown skill",
	"not_enough_resource": "Not enough energy",
	"stunned": "You are stunned",
}

## Test options.
var autowalk := false
var autofight := false

var my_id := 0
var zone_id := ""
var tick_rate := 20
var move_speed := 5.0
var half_extent := 60.0
var _terrain  # Terrain, or null on a flat zone
var _hud_warning := ""

var _entities := {}  # id -> Character
var _client_tick := 0.0
var _clock_synced := false

var _pending: Array[Dictionary] = []  # unacknowledged inputs: {seq, dir: Vector2}
var _seq := 0
var _acc := 0.0
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
var _weapon: PB.WeaponInfo
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
	_rules = welcome.action_rules
	_weapon = welcome.weapon
	_load_terrain(welcome.terrain)
	_build_environment()
	_build_camera()
	_hud = Hud.new()
	add_child(_hud)
	_hud.set_skills(welcome.skills)
	_hud.set_class(welcome.class_info)
	if not _hud_warning.is_empty():
		_hud.show_message(_hud_warning, Color(1, 0.4, 0.35))
	_hud.skill_pressed.connect(use_skill)
	_build_ring()
	if welcome.status != null:
		_on_status(welcome.status)
	_hud.set_inventory(welcome.inventory)
	Net.snapshot_received.connect(_on_snapshot)
	Net.skill_rejected.connect(_on_skill_rejected)
	Net.fate_received.connect(_on_fate)
	if not autofight and not autowalk:
		_capture_mouse(true)


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
		local.set_action(_act, _act_end if _act in [A_SWING, A_FLOURISH, A_ROLL] else 0)

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

const BTN_ATTACK := 1
const BTN_BLOCK := 2
const BTN_ROLL := 4
const A_IDLE := 0
const A_SWING := 1
const A_FLOURISH := 2
const A_CHARGING := 3
const A_BLOCK := 4
const A_ROLL := 5

# Action state machine, mirroring zone/action.go stepAction with the timings
# from Welcome.action_rules. Timers count input sequence numbers.
var _rules: PB.ActionRules
var _act := A_IDLE
var _act_end := 0
var _charge_seq := 0
var _roll_ready := 0
var _roll_dir := Vector2.ZERO
var _queued_attack := false
var _prev_buttons := 0
var _roll_pressed := false  # latched until the next input goes out
var _auto_click := false


func _read_input() -> Vector2:
	if _is_dead():
		return Vector2.ZERO
	if autofight:
		var target: Character = _entities.get(_target_id)
		if target != null and target.is_alive():
			var to := Vector2(target.global_position.x - _pred_pos.x, target.global_position.z - _pred_pos.z)
			return to.normalized() if to.length() > _autofight_range() else Vector2.ZERO
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


func _buttons() -> int:
	var b := 0
	if _roll_pressed:
		b |= BTN_ROLL
	_roll_pressed = false
	if _is_dead():
		return 0
	if autofight:
		if _autofight_wants_attack():
			# Click: hold for one input, release for the next.
			_auto_click = not _auto_click
			if _auto_click:
				b |= BTN_ATTACK
		return b
	if Input.mouse_mode == Input.MOUSE_MODE_CAPTURED and get_window().has_focus():
		if Input.is_mouse_button_pressed(MOUSE_BUTTON_LEFT):
			b |= BTN_ATTACK
		if Input.is_mouse_button_pressed(MOUSE_BUTTON_RIGHT):
			b |= BTN_BLOCK
	return b


## Where the character faces: the locked target, else the direction of the
## camera while fighting, else where it walks.
func _facing(dir: Vector2, buttons: int) -> float:
	var target: Character = _entities.get(_target_id)
	if target != null and target.is_alive():
		return atan2(target.global_position.x - _pred_pos.x, target.global_position.z - _pred_pos.z)
	if buttons & (BTN_ATTACK | BTN_BLOCK) or _act in [A_SWING, A_FLOURISH, A_CHARGING, A_BLOCK]:
		return _cam_yaw + PI
	if dir.length_squared() > 0.0001:
		return atan2(dir.x, dir.y)
	return _pred_yaw


## Mirrors zone/action.go stepAction. Returns [speed factor, movement dir].
func _step_action(seq: int, buttons: int, dir: Vector2) -> Array:
	var prev := _prev_buttons
	_prev_buttons = buttons
	var held := buttons & BTN_ATTACK != 0
	var pressed := held and prev & BTN_ATTACK == 0
	var released := not held and prev & BTN_ATTACK != 0
	var busy := _act == A_SWING or _act == A_FLOURISH
	if busy and pressed:
		_queued_attack = true
	if (busy or _act == A_ROLL) and seq >= _act_end:
		if _act != A_ROLL and _queued_attack:
			_queued_attack = false
			_start_swing(seq, false)
		elif _act != A_ROLL and held:
			_act = A_CHARGING
			_charge_seq = seq
		else:
			_act = A_IDLE
		busy = _act == A_SWING or _act == A_FLOURISH
	if buttons & BTN_ROLL and not busy and _act != A_ROLL and seq >= _roll_ready:
		if dir.length() > 0.01:
			_roll_dir = dir.normalized()
		else:
			_roll_dir = Vector2(-sin(_pred_yaw), -cos(_pred_yaw))
		_act_end = seq + _rules.roll_ticks
		_roll_ready = seq + _rules.roll_cooldown_ticks
		_queued_attack = false
		_act = A_ROLL
	match _act:
		A_IDLE, A_BLOCK:
			if pressed:
				_start_swing(seq, false)
			elif buttons & BTN_BLOCK:
				_act = A_BLOCK
			else:
				_act = A_IDLE
		A_CHARGING:
			if released:
				if seq - _charge_seq >= _rules.charge_ticks:
					_start_swing(seq, true)
				else:
					_act = A_IDLE
	match _act:
		A_ROLL:
			return [_rules.roll_speed, _roll_dir]
		A_BLOCK:
			return [_rules.block_speed, dir]
		A_SWING, A_FLOURISH:
			return [_rules.swing_speed, dir]
		A_CHARGING:
			return [_rules.charge_speed, dir]
	return [1.0, dir]


func _start_swing(seq: int, flourish: bool) -> void:
	_act = A_FLOURISH if flourish else A_SWING
	_act_end = seq + (_rules.flourish_ticks if flourish else _rules.swing_ticks)


func _step_input() -> void:
	var dir := _read_input()
	var buttons := _buttons()
	_seq += 1
	var yaw := _facing(dir, buttons)
	var res := _step_action(_seq, buttons, dir) if _rules != null else [1.0, dir]
	var speed: float = res[0]
	var mdir: Vector2 = res[1]
	_pred_yaw = yaw
	_prev_pred_pos = _pred_pos
	# Every tick: the server runs the same state machine on these inputs.
	Net.send_move(_seq, dir.x, dir.y, yaw, buttons, _target_id)
	_pending.append({"seq": _seq, "dir": mdir, "speed": speed})
	_pred_pos = _simulate(_pred_pos, mdir, speed)
	_pred_moving = mdir.length_squared() > 0.0001


## Must match zone.applyInputs on the server.
func _simulate(pos: Vector3, dir: Vector2, speed := 1.0) -> Vector3:
	if dir.length() > 1.0:
		dir = dir.normalized()
	var step := move_speed * speed / tick_rate
	# Same order as the server (zone.walk): clamp to bounds, then terrain rules.
	var tx := clampf(pos.x + dir.x * step, -half_extent, half_extent)
	var tz := clampf(pos.z + dir.y * step, -half_extent, half_extent)
	if _terrain == null:
		return Vector3(tx, pos.y, tz)
	var p: Vector2 = _terrain.move(pos.x, pos.z, tx - pos.x, tz - pos.z)
	return Vector3(p.x, _terrain.height(p.x, p.y), p.y)


func _reconcile(server_pos: Vector3, ack: int) -> void:
	while not _pending.is_empty() and _pending[0]["seq"] <= ack:
		_pending.pop_front()
	var corrected := server_pos
	for input in _pending:
		corrected = _simulate(corrected, input["dir"], input["speed"])
	var error := corrected - _pred_pos
	if error.length() > SNAP_DISTANCE:
		_prev_pred_pos = corrected
	else:
		_prev_pred_pos += error
	_pred_pos = corrected


# --- Targeting and skills ----------------------------------------------------

func _capture_mouse(on: bool) -> void:
	Input.mouse_mode = Input.MOUSE_MODE_CAPTURED if on else Input.MOUSE_MODE_VISIBLE


func _unhandled_input(event: InputEvent) -> void:
	var captured := Input.mouse_mode == Input.MOUSE_MODE_CAPTURED
	if event is InputEventMouseMotion and captured:
		_cam_yaw -= event.relative.x * 0.004
		_cam_pitch = clampf(_cam_pitch + event.relative.y * 0.003, 0.05, 1.3)
	elif event is InputEventMouseButton and event.pressed:
		match event.button_index:
			MOUSE_BUTTON_WHEEL_UP:
				_cam_distance = maxf(_cam_distance - 0.6, 2.5)
			MOUSE_BUTTON_WHEEL_DOWN:
				_cam_distance = minf(_cam_distance + 0.6, 25.0)
			MOUSE_BUTTON_MIDDLE:
				_toggle_lock()
			MOUSE_BUTTON_LEFT:
				if not captured:
					_capture_mouse(true)  # click back into the game (no attack)
					get_viewport().set_input_as_handled()
	elif event is InputEventKey and event.pressed and not event.echo:
		match event.physical_keycode:
			KEY_ESCAPE:
				if captured:
					_capture_mouse(false)  # free the cursor; Esc again logs out
					get_viewport().set_input_as_handled()
			KEY_SPACE:
				_roll_pressed = true
			KEY_F:
				_toggle_lock()
			KEY_TAB:
				_cycle_target()
				get_viewport().set_input_as_handled()
			KEY_I:
				_hud.toggle_inventory()
			KEY_1, KEY_2, KEY_3, KEY_4:
				var i: int = event.physical_keycode - KEY_1
				if i < _skill_order.size():
					use_skill(_skill_order[i])


## F or middle mouse: lock onto the enemy nearest the center of view, or unlock.
func _toggle_lock() -> void:
	if _target_id != 0:
		_set_target(0)
		return
	var best := 0
	var best_score := INF
	var forward := Vector2(-sin(_cam_yaw), -cos(_cam_yaw))
	for id in _entities:
		var c: Character = _entities[id]
		if not c.is_npc() or not c.is_alive():
			continue
		var to := Vector2(c.global_position.x - _pred_pos.x, c.global_position.z - _pred_pos.z)
		var d := to.length()
		if d > 25.0 or d < 0.01:
			continue
		var angle := absf(forward.angle_to(to / d))
		if angle > 1.2:
			continue
		var score := d * (1.0 + angle * 2.0)
		if score < best_score:
			best = id
			best_score = score
	if best == 0:
		_hud.show_message("No enemy in view", Color(0.8, 0.8, 0.8))
	_set_target(best)


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
	if _status != null and _status.resource < s.cost:
		_hud.show_message("Not enough energy", Color(0.6, 0.75, 1.0))
		return
	if s.target_self:
		if Time.get_ticks_msec() < _ready_at.get(skill_id, 0):
			_hud.show_message("Not ready yet", Color(0.8, 0.8, 0.8))
			return
		_ready_at[skill_id] = Time.get_ticks_msec() + s.cooldown_ms
		Net.send_skill(skill_id, 0)
		return
	var target: Character = _entities.get(_target_id)
	if target == null or not target.is_npc() or not target.is_alive():
		target = _enemy_in_front(s.range + 0.5)
	if target == null:
		_hud.show_message("No enemy in front (F to lock on)", Color(1, 0.85, 0.5))
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
	Net.send_skill(skill_id, target.entity_id)


func _on_skill_rejected(skill_id: int, reason: String) -> void:
	_ready_at.erase(skill_id)  # let the player retry right away
	_hud.show_message(REJECT_TEXT.get(reason, reason), Color(1, 0.6, 0.5))


## The living enemy closest to where the character faces, within range.
func _enemy_in_front(max_range: float) -> Character:
	var best: Character = null
	var best_score := INF
	var forward := Vector2(sin(_pred_yaw), cos(_pred_yaw))
	for id in _entities:
		var c: Character = _entities[id]
		if not c.is_npc() or not c.is_alive():
			continue
		var to := Vector2(c.global_position.x - _pred_pos.x, c.global_position.z - _pred_pos.z)
		var d := to.length()
		if d > max_range or d < 0.01:
			continue
		var angle := absf(forward.angle_to(to / d))
		if angle > 1.0:
			continue
		var score := d * (1.0 + angle)
		if score < best_score:
			best = c
			best_score = score
	return best


## How close autofight walks: well within the weapon's reach.
func _autofight_range() -> float:
	if _weapon == null:
		return 1.8
	return maxf(1.5, _weapon.range * (0.7 if _weapon.kind == "melee" else 0.6))


## Autofight attacks while the locked target is within weapon reach.
func _autofight_wants_attack() -> bool:
	var target: Character = _entities.get(_target_id)
	if target == null or not target.is_alive() or _weapon == null:
		return false
	return target.global_position.distance_to(_pred_pos) <= _weapon.range + 0.3


## Test option: target the nearest enemy, walk to it and use skills.
func _autofight() -> void:
	var target: Character = _entities.get(_target_id)
	if target == null or not target.is_alive():
		_cycle_target()
		return
	# Roll away from telegraphed attacks, like a player would.
	if target.action in [6, 7] and target.global_position.distance_to(_pred_pos) < 4.0 and randf() < 0.5:
		_roll_pressed = true
	var now := Time.get_ticks_msec()
	for id in _skill_order:
		var s: PB.SkillInfo = _skills[id]
		if now < _ready_at.get(id, 0) or (_status != null and _status.resource < s.cost):
			continue
		if s.target_self:
			if _status != null and _status.hp < _status.max_hp * 0.6:
				use_skill(id)
		elif target.global_position.distance_to(_pred_pos) <= s.range:
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
	if state.id != my_id:
		c.set_action(state.action, state.action_tick)
	if state.id == my_id:
		_reconcile(_vec(state.pos), snap.ack_input_seq)
	else:
		c.push_state(snap.tick, _vec(state.pos), state.yaw, state.moving)


func _on_combat_event(ev: PB.CombatEvent) -> void:
	var src: Character = _entities.get(ev.source_id)
	var dst: Character = _entities.get(ev.target_id)
	match ev.kind:
		PB.CombatEventKind.COMBAT_EVENT_KIND_DAMAGE:
			var look := _attack_look(src, ev)
			if src != null and src != dst and not ev.weapon and not src.is_npc():
				src.play_action(look["anim"])
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
			# Look the target up on arrival: it may have despawned meanwhile.
			var target_id := ev.target_id
			var critical := ev.critical
			var land := func():
				var d: Character = _entities.get(target_id)
				if d == null:
					return
				_land_hit(d, amount, color)
				VFX.land(self, look["vfx"], d.global_position, look["radius"], critical)
			# Damage over time (source == target or repeated ticks) has no cast.
			if src == null or src == dst or not VFX.cast(self, look["vfx"], src, dst, look["projectile"], land):
				land.call()
		PB.CombatEventKind.COMBAT_EVENT_KIND_HEAL:
			var heal_skill: PB.SkillInfo = _skills.get(ev.skill_id)
			if src != null and src == dst and heal_skill != null:
				src.play_action(heal_skill.anim)
			if dst != null:
				dst.show_number("+%d" % ev.amount, Color(0.45, 1.0, 0.5))
				VFX.heal(self, dst.global_position)
		PB.CombatEventKind.COMBAT_EVENT_KIND_BLOCKED:
			if dst != null:
				dst.show_number("Block %d" % ev.amount, Color(0.6, 0.8, 1.0))
				VFX.impact(self, dst.global_position + Vector3(0, 1.2, 0), "ward")
		PB.CombatEventKind.COMBAT_EVENT_KIND_DODGED:
			if dst != null:
				dst.show_number("Dodge", Color(0.85, 0.95, 1.0))
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


## How an attack looks: the attacker's weapon, a skill, or the NPC's attack.
func _attack_look(src: Character, ev: PB.CombatEvent) -> Dictionary:
	var skill_id := ev.skill_id
	var look := {"anim": NPC_ATTACK_ANIM, "vfx": "hit", "projectile": false, "radius": 0.0}
	if ev.weapon and src != null:
		var w := ContentDB.weapon_for(src.class_key)
		look["vfx"] = w.get("vfx", "hit")
		look["projectile"] = w.get("kind", "melee") == "ranged"
		if ev.flourish:
			look["radius"] = 2.5
		return look
	if src != null and src.is_npc():
		if not src.attack_anim.is_empty():
			look["anim"] = src.attack_anim
		if not src.attack_vfx.is_empty():
			look["vfx"] = src.attack_vfx
		look["projectile"] = src.attack_projectile
		return look
	var s := ContentDB.skill(skill_id)
	if not s.is_empty():
		look["anim"] = s.get("anim", NPC_ATTACK_ANIM)
		look["vfx"] = s.get("vfx", "hit")
		look["projectile"] = s.get("projectile", false)
		look["radius"] = float(s.get("radius", 0.0))
	return look


func _on_fate(ev: PB.FateEvent) -> void:
	var me: Character = _entities.get(my_id)
	if me != null:
		var key := "fate_fortune" if ev.omen == PB.FateOmen.FATE_OMEN_FORTUNE else "fate_misfortune"
		VFX.burst(self, me.global_position + Vector3(0, 1.0, 0), VFX.color(key), 30 + 15 * ev.magnitude, 2.5, 0.06, 1.0)
		VFX.flash(self, me.global_position + Vector3(0, 1.5, 0), VFX.color(key), 3.0 + ev.magnitude, 6.0)
		if ev.action == "ambush":
			VFX.burst(self, me.global_position, Color(0.15, 0.12, 0.18, 0.8), 60, 6.0, 0.25, -1.0)
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
	if _terrain != null:
		# Keep the camera above hills behind the player.
		_camera.position.y = maxf(_camera.position.y, _terrain.height(_camera.position.x, _camera.position.z) + 0.6)
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
	var affordable := {}
	for id in _skills:
		affordable[id] = _status == null or _status.resource >= _skills[id].cost
	_hud.update_cooldowns(remaining, affordable)


func _update_hud() -> void:
	_hud.debug_label.text = "%s  ·  entity %d  ·  visible %d  ·  rtt %d ms\nWASD move · LMB attack (hold: flourish) · RMB block · Space roll · F lock · 1-4 skills · I inventory · Esc cursor" % [
		zone_id, my_id, _entities.size(), Net.rtt_ms]
	_hud.set_crosshair(_target_id == 0 and Input.mouse_mode == Input.MOUSE_MODE_CAPTURED)
	_hud.set_charging(_act == A_CHARGING and _seq - _charge_seq >= (_rules.charge_ticks if _rules else 8))


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

	if _terrain != null:
		_build_terrain()
		return

	# Flat zone: ground with a 2 m grid so movement reads clearly.
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


## Loads the zone heightmap named in Welcome and checks it matches the server's.
func _load_terrain(info: PB.TerrainInfo) -> void:
	if info == null:
		return
	_terrain = Terrain.load_file("res://assets/terrain/" + info.file)
	if _terrain == null:
		_hud_warning = "Terrain file %s is missing; movement will stutter." % info.file
		return
	_terrain.max_slope = info.max_slope
	_terrain.water_level = info.water_level
	_terrain.wade_depth = info.wade_depth
	if _terrain.sha256 != info.sha256:
		_hud_warning = "Your terrain files are out of date; update the client."
		push_warning("terrain hash mismatch: client %s, server %s" % [_terrain.sha256, info.sha256])


func _build_terrain() -> void:
	var ground := MeshInstance3D.new()
	ground.mesh = _terrain.build_mesh()
	var shader := Shader.new()
	shader.code = """
shader_type spatial;
render_mode cull_disabled;
uniform float max_slope = 0.84;
uniform float water_level = -1.0;
varying vec3 world_pos;
varying vec3 world_normal;
float hash(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }
void vertex() {
	world_pos = (MODEL_MATRIX * vec4(VERTEX, 1.0)).xyz;
	world_normal = NORMAL;
}
void fragment() {
	vec3 n = normalize(world_normal);
	float slope = sqrt(max(0.0, 1.0 - n.y * n.y)) / max(n.y, 0.001);
	float speckle = hash(floor(world_pos.xz * 0.8)) * 0.035 - 0.0175;
	vec3 grass = mix(vec3(0.27, 0.44, 0.25), vec3(0.45, 0.55, 0.30), clamp(world_pos.y / 14.0, 0.0, 1.0));
	vec3 rock = vec3(0.47, 0.45, 0.41);
	vec3 sand = vec3(0.68, 0.62, 0.47);
	vec3 c = mix(grass, rock, smoothstep(max_slope * 0.7, max_slope * 1.05, slope));
	c = mix(c, sand, 1.0 - smoothstep(water_level + 0.15, water_level + 0.8, world_pos.y));
	// Colors above are sRGB; the renderer works in linear space.
	ALBEDO = pow(max(c + speckle, vec3(0.0)), vec3(2.2));
	ROUGHNESS = 0.95;
}
"""
	var mat := ShaderMaterial.new()
	mat.shader = shader
	mat.set_shader_parameter("max_slope", _terrain.max_slope)
	mat.set_shader_parameter("water_level", _terrain.water_level)
	ground.material_override = mat
	add_child(ground)

	var water := MeshInstance3D.new()
	var plane := PlaneMesh.new()
	plane.size = Vector2(_terrain.extent(), _terrain.extent())
	water.mesh = plane
	water.position = Vector3(_terrain.origin_x + _terrain.extent() / 2.0, _terrain.water_level, _terrain.origin_z + _terrain.extent() / 2.0)
	var wmat := StandardMaterial3D.new()
	wmat.albedo_color = Color(0.18, 0.42, 0.55, 0.65)
	wmat.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
	wmat.roughness = 0.08
	wmat.metallic = 0.3
	water.material_override = wmat
	add_child(water)
