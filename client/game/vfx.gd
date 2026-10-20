## Visual effects, built from primitives in code (no assets): projectiles,
## impacts, slashes, explosions, healing, pillars of light. Skills name an
## effect with their "vfx" key (content/skills.yaml); NPC attacks too.
extends RefCounted

const COLORS := {
	"slash": Color(0.6, 1.0, 0.8),
	"wave": Color(0.45, 1.0, 0.75),
	"ward": Color(0.6, 0.85, 1.0),
	"bullet": Color(1.0, 0.85, 0.4),
	"beam": Color(0.4, 0.9, 1.0),
	"explosion": Color(1.0, 0.55, 0.15),
	"coil": Color(0.35, 1.0, 0.9),
	"lance": Color(1.0, 0.95, 0.7),
	"smite": Color(1.0, 0.9, 0.35),
	"heal": Color(0.45, 1.0, 0.5),
	"mark": Color(0.75, 0.45, 1.0),
	"wisp_bolt": Color(0.45, 1.0, 0.7),
	"hit": Color(1.0, 0.95, 0.85),
	"fate_fortune": Color(1.0, 0.82, 0.35),
	"fate_misfortune": Color(0.6, 0.4, 0.9),
}

# Projectile size and speed (m/s) by effect.
const PROJECTILES := {
	"bullet": [0.07, 45.0],
	"beam": [0.22, 55.0],
	"wave": [0.3, 22.0],
	"explosion": [0.16, 24.0],
	"coil": [0.14, 26.0],
	"lance": [0.12, 34.0],
	"wisp_bolt": [0.14, 18.0],
}


static func color(key: String) -> Color:
	return COLORS.get(key, COLORS["hit"])


static func _unshaded(c: Color) -> StandardMaterial3D:
	var m := StandardMaterial3D.new()
	m.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	m.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
	m.albedo_color = c
	m.cull_mode = BaseMaterial3D.CULL_DISABLED
	return m


static func _free_after(node: Node, seconds: float) -> void:
	node.get_tree().create_timer(seconds).timeout.connect(node.queue_free)


## A burst of small glowing particles.
static func burst(parent: Node, at: Vector3, c: Color, amount := 16, speed := 3.0, size := 0.05, gravity := -4.0) -> void:
	var p := GPUParticles3D.new()
	var pm := ParticleProcessMaterial.new()
	pm.direction = Vector3.UP
	pm.spread = 180.0
	pm.initial_velocity_min = speed * 0.4
	pm.initial_velocity_max = speed
	pm.gravity = Vector3(0, gravity, 0)
	pm.scale_min = 0.6
	pm.scale_max = 1.2
	p.process_material = pm
	var dot := SphereMesh.new()
	dot.radius = size
	dot.height = size * 2.0
	dot.material = _unshaded(c)
	p.draw_pass_1 = dot
	p.amount = amount
	p.lifetime = 0.7
	p.one_shot = true
	p.explosiveness = 0.95
	parent.add_child(p)
	p.global_position = at
	p.emitting = true
	_free_after(p, 1.5)


## A flash of light at a point.
static func flash(parent: Node, at: Vector3, c: Color, energy := 3.0, radius := 4.0) -> void:
	var l := OmniLight3D.new()
	l.light_color = c
	l.light_energy = energy
	l.omni_range = radius
	parent.add_child(l)
	l.global_position = at
	var tw := l.create_tween()
	tw.tween_property(l, "light_energy", 0.0, 0.35)
	tw.tween_callback(l.queue_free)


## A hit landing: sparks and a flash.
static func impact(parent: Node, at: Vector3, key: String, big := false) -> void:
	var c := color(key)
	burst(parent, at, c, 26 if big else 12, 4.5 if big else 3.0)
	flash(parent, at, c, 4.0 if big else 2.0)


## A crescent arc swept in front of the attacker.
static func slash(parent: Node, from: Node3D, key: String) -> void:
	var arc := MeshInstance3D.new()
	var torus := TorusMesh.new()
	torus.inner_radius = 0.9
	torus.outer_radius = 1.05
	arc.mesh = torus
	arc.material_override = _unshaded(Color(color(key), 0.85))
	parent.add_child(arc)
	arc.global_position = from.global_position + Vector3(0, 1.1, 0)
	arc.rotation = Vector3(deg_to_rad(80), from.rotation.y, 0)
	arc.scale = Vector3(1, 0.2, 1)
	var tw := arc.create_tween().set_parallel(true)
	tw.tween_property(arc, "rotation:y", from.rotation.y + PI * 0.8, 0.2)
	tw.tween_property(arc.material_override, "albedo_color:a", 0.0, 0.25)
	tw.chain().tween_callback(arc.queue_free)


## An orb (with a fading trail) flying from one point to another; calls
## on_hit when it arrives.
static func projectile(parent: Node, from: Vector3, to: Vector3, key: String, on_hit: Callable) -> void:
	var spec: Array = PROJECTILES.get(key, [0.12, 30.0])
	var c := color(key)
	var orb := MeshInstance3D.new()
	var sphere := SphereMesh.new()
	sphere.radius = spec[0]
	sphere.height = spec[0] * 2.0
	orb.mesh = sphere
	var mat := _unshaded(c)
	mat.emission_enabled = true
	mat.emission = c
	orb.material_override = mat
	parent.add_child(orb)
	orb.global_position = from
	var light := OmniLight3D.new()
	light.light_color = c
	light.light_energy = 1.2
	light.omni_range = 2.5
	orb.add_child(light)
	var trail := GPUParticles3D.new()
	var pm := ParticleProcessMaterial.new()
	pm.gravity = Vector3.ZERO
	pm.scale_min = 0.5
	pm.scale_max = 0.9
	trail.process_material = pm
	var dot := SphereMesh.new()
	dot.radius = spec[0] * 0.6
	dot.height = spec[0] * 1.2
	dot.material = _unshaded(Color(c, 0.6))
	trail.draw_pass_1 = dot
	trail.amount = 24
	trail.lifetime = 0.25
	trail.local_coords = false
	orb.add_child(trail)
	var t := clampf(from.distance_to(to) / spec[1], 0.05, 1.2)
	var tw := orb.create_tween()
	tw.tween_property(orb, "global_position", to, t)
	tw.tween_callback(on_hit)
	tw.tween_callback(orb.queue_free)


## An expanding sphere of fire and sparks.
static func explosion(parent: Node, at: Vector3, radius: float, key := "explosion") -> void:
	var c := color(key)
	var ball := MeshInstance3D.new()
	var sphere := SphereMesh.new()
	ball.mesh = sphere
	ball.material_override = _unshaded(Color(c, 0.55))
	parent.add_child(ball)
	ball.global_position = at
	ball.scale = Vector3.ONE * 0.2
	var tw := ball.create_tween().set_parallel(true)
	tw.tween_property(ball, "scale", Vector3.ONE * radius * 2.0, 0.3).set_ease(Tween.EASE_OUT)
	tw.tween_property(ball.material_override, "albedo_color:a", 0.0, 0.4)
	tw.chain().tween_callback(ball.queue_free)
	burst(parent, at, c, 40, 7.0, 0.07)
	flash(parent, at, c, 6.0, radius * 2.5)


## A ring on the ground spreading out (area hits, slams).
static func shockwave(parent: Node, at: Vector3, radius: float, key: String) -> void:
	var ring := MeshInstance3D.new()
	var torus := TorusMesh.new()
	torus.inner_radius = 0.9
	torus.outer_radius = 1.0
	ring.mesh = torus
	ring.material_override = _unshaded(Color(color(key), 0.8))
	parent.add_child(ring)
	ring.global_position = at + Vector3(0, 0.1, 0)
	ring.scale = Vector3(0.3, 0.05, 0.3)
	var tw := ring.create_tween().set_parallel(true)
	tw.tween_property(ring, "scale", Vector3(radius, 0.05, radius), 0.35).set_ease(Tween.EASE_OUT)
	tw.tween_property(ring.material_override, "albedo_color:a", 0.0, 0.4)
	tw.chain().tween_callback(ring.queue_free)


## A column of light from the sky (smites, blessings).
static func pillar(parent: Node, at: Vector3, key: String, height := 8.0) -> void:
	var c := color(key)
	var col := MeshInstance3D.new()
	var cyl := CylinderMesh.new()
	cyl.top_radius = 0.5
	cyl.bottom_radius = 0.5
	cyl.height = height
	cyl.cap_top = false
	cyl.cap_bottom = false
	col.mesh = cyl
	col.material_override = _unshaded(Color(c, 0.5))
	parent.add_child(col)
	col.global_position = at + Vector3(0, height / 2.0, 0)
	var tw := col.create_tween().set_parallel(true)
	tw.tween_property(col, "scale", Vector3(0.2, 1, 0.2), 0.5)
	tw.tween_property(col.material_override, "albedo_color:a", 0.0, 0.5)
	tw.chain().tween_callback(col.queue_free)
	flash(parent, at + Vector3(0, 1, 0), c, 5.0, 6.0)


## Healing: green motes rising around someone.
static func heal(parent: Node, at: Vector3) -> void:
	burst(parent, at + Vector3(0, 0.3, 0), color("heal"), 24, 1.6, 0.05, 2.5)
	flash(parent, at + Vector3(0, 1, 0), color("heal"), 2.0)


## A glyph that hovers over a marked target and fades.
static func mark(parent: Node, target: Node3D, key := "mark") -> void:
	var ring := MeshInstance3D.new()
	var torus := TorusMesh.new()
	torus.inner_radius = 0.35
	torus.outer_radius = 0.45
	ring.mesh = torus
	ring.material_override = _unshaded(color(key))
	target.add_child(ring)
	ring.position = Vector3(0, 2.6, 0)
	ring.rotation.x = PI / 2
	var tw := ring.create_tween()
	tw.tween_property(ring, "rotation:z", TAU, 1.2)
	tw.tween_property(ring.material_override, "albedo_color:a", 0.0, 0.4)
	tw.tween_callback(ring.queue_free)


## Plays a skill's cast-side effect and returns true if it handles the
## travel to the target itself (projectile), calling on_hit on arrival.
static func cast(parent: Node, key: String, src: Node3D, dst: Node3D, projectile: bool, on_hit: Callable) -> bool:
	if src == null:
		return false
	match key:
		"slash":
			slash(parent, src, key)
		"ward":
			shockwave(parent, src.global_position, 1.6, key)
	if projectile and dst != null:
		projectile(parent, src.global_position + Vector3(0, 1.2, 0), dst.global_position + Vector3(0, 1.2, 0), key, on_hit)
		return true
	return false


## The effect where a skill lands.
static func land(parent: Node, key: String, at: Vector3, radius: float, critical: bool) -> void:
	match key:
		"explosion":
			explosion(parent, at, maxf(radius, 1.5))
		"wave":
			shockwave(parent, at, maxf(radius, 2.0), key)
			impact(parent, at + Vector3(0, 1, 0), key, critical)
		"smite":
			pillar(parent, at, key)
		_:
			impact(parent, at + Vector3(0, 1.1, 0), key if COLORS.has(key) else "hit", critical)
