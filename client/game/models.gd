## Character model registry. All models share the Quaternius UAL skeleton, so
## one animation library (from UAL1) drives every model; tracks are re-pointed
## to each model's own Skeleton3D path once and cached.
extends RefCounted

# Animation libraries on the shared skeleton: UAL1 (locomotion, basic
# combat) and UAL2 (sword combos, blocks, rolls and more), merged.
const ANIMATION_SOURCES := [
	"res://assets/characters/mannequin_m/UAL1_Standard.glb",
	"res://assets/characters/mannequin_m/UAL2_Standard.glb",
]

const MODELS := {
	"mannequin_m": {
		"label": "Mannequin (M)",
		"scene": "res://assets/characters/mannequin_m/UAL1_Standard.glb",
	},
	"mannequin_f": {
		"label": "Mannequin (F)",
		"scene": "res://assets/characters/mannequin_f/Mannequin_F.glb",
	},
	"zuo_mo": {
		"label": "Zuo Mo",
		"scene": "res://assets/characters/zuo_mo/zuo_mo.glb",
		# The source sculpt has no texture.
		"albedo": Color(0.36, 0.40, 0.46),
	},
	"bandit": {
		"label": "Bandit",
		"scene": "res://assets/characters/mannequin_m/UAL1_Standard.glb",
		"albedo": Color(0.45, 0.16, 0.12),
		"npc_only": true,
	},
	"golem": {
		"label": "Stone Guardian",
		"scene": "res://assets/characters/mannequin_m/UAL1_Standard.glb",
		"albedo": Color(0.46, 0.45, 0.41),
		"roughness": 1.0,
		"scale": 1.7,
		"anim_speed": 0.65,
		"npc_only": true,
	},
	"shade": {
		"label": "Shadow Assassin",
		"scene": "res://assets/characters/mannequin_f/Mannequin_F.glb",
		"albedo": Color(0.1, 0.06, 0.16, 0.8),
		"emission": Color(0.25, 0.05, 0.35),
		"anim_speed": 1.3,
		"npc_only": true,
	},
	"wisp": {
		"label": "Jade Wisp",
		# No mesh: a glowing orb built in code.
		"procedural": "wisp",
		"color": Color(0.45, 1.0, 0.7),
		"npc_only": true,
	},
	"villager": {
		"label": "Villager",
		"scene": "res://assets/characters/mannequin_m/UAL1_Standard.glb",
		"albedo": Color(0.55, 0.45, 0.3),
		"npc_only": true,
	},
	"elder": {
		"label": "Elder",
		"scene": "res://assets/characters/mannequin_f/Mannequin_F.glb",
		"albedo": Color(0.2, 0.3, 0.5),
		"npc_only": true,
	},
	"old_black": {
		"label": "Old Black",
		"scene": "res://assets/characters/old_black/old_black.glb",
		"albedo": Color(0.22, 0.21, 0.24),
		"npc_only": true,
	},
}

const DEFAULT_MODEL := "mannequin_m"

static var _source_lib: AnimationLibrary
static var _source_skeleton_path := ""
static var _libs := {}  # model key -> AnimationLibrary retargeted for that model


## Models a player can pick for a new character.
static func keys() -> Array:
	return MODELS.keys().filter(func(k): return not MODELS[k].get("npc_only", false))


static func label(key: String) -> String:
	return MODELS.get(key, MODELS[DEFAULT_MODEL])["label"]


## Instantiates the model and an AnimationPlayer wired to its skeleton.
## Returns [model_root: Node3D, player: AnimationPlayer or null].
static func instantiate(key: String) -> Array:
	if not MODELS.has(key):
		key = DEFAULT_MODEL
	var info: Dictionary = MODELS[key]
	if info.has("procedural"):
		return [_build_wisp(info["color"]), null]
	var root: Node3D = (load(info["scene"]) as PackedScene).instantiate()
	root.scale = Vector3.ONE * float(info.get("scale", 1.0))

	# Drop any player the glTF brought along; we add our own below.
	for ap in root.find_children("*", "AnimationPlayer", true, false):
		ap.get_parent().remove_child(ap)
		ap.queue_free()

	if info.has("albedo"):
		var mat := StandardMaterial3D.new()
		mat.albedo_color = info["albedo"]
		mat.roughness = float(info.get("roughness", 0.85))
		if mat.albedo_color.a < 1.0:
			mat.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
		if info.has("emission"):
			mat.emission_enabled = true
			mat.emission = info["emission"]
		for mi in root.find_children("*", "MeshInstance3D", true, false):
			(mi as MeshInstance3D).material_override = mat

	var skeleton := _find_skeleton(root)
	var player := AnimationPlayer.new()
	root.add_child(player)
	player.root_node = NodePath("..")
	if skeleton != null:
		player.add_animation_library("", _library_for(key, String(root.get_path_to(skeleton))))
	player.speed_scale = float(info.get("anim_speed", 1.0))
	return [root, player]


## A spirit: a glowing core, a soft halo, a light, and drifting motes.
static func _build_wisp(color: Color) -> Node3D:
	var root := Node3D.new()
	var body := Node3D.new()
	body.name = "Body"
	body.position = Vector3(0, 1.3, 0)
	root.add_child(body)
	for layer in [[0.22, 1.0], [0.42, 0.25]]:
		var orb := MeshInstance3D.new()
		var sphere := SphereMesh.new()
		sphere.radius = layer[0]
		sphere.height = layer[0] * 2.0
		orb.mesh = sphere
		var mat := StandardMaterial3D.new()
		mat.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
		mat.transparency = BaseMaterial3D.TRANSPARENCY_ALPHA
		mat.albedo_color = Color(color, layer[1])
		orb.material_override = mat
		body.add_child(orb)
	var light := OmniLight3D.new()
	light.light_color = color
	light.light_energy = 1.5
	light.omni_range = 4.0
	body.add_child(light)
	var motes := GPUParticles3D.new()
	var pm := ParticleProcessMaterial.new()
	pm.emission_shape = ParticleProcessMaterial.EMISSION_SHAPE_SPHERE
	pm.emission_sphere_radius = 0.35
	pm.gravity = Vector3(0, 0.4, 0)
	pm.initial_velocity_min = 0.05
	pm.initial_velocity_max = 0.2
	pm.scale_min = 0.5
	pm.scale_max = 1.0
	motes.process_material = pm
	motes.amount = 16
	motes.lifetime = 1.4
	var dot := SphereMesh.new()
	dot.radius = 0.03
	dot.height = 0.06
	var dmat := StandardMaterial3D.new()
	dmat.shading_mode = BaseMaterial3D.SHADING_MODE_UNSHADED
	dmat.albedo_color = color
	dot.material = dmat
	motes.draw_pass_1 = dot
	body.add_child(motes)
	return root  # Character starts the bobbing once it is in the scene tree


static func _find_skeleton(root: Node) -> Skeleton3D:
	var found := root.find_children("*", "Skeleton3D", true, false)
	return found[0] if not found.is_empty() else null


static func _library_for(key: String, skeleton_path: String) -> AnimationLibrary:
	if _libs.has(key):
		return _libs[key]
	_load_source()
	var lib := AnimationLibrary.new()
	for anim_name in _source_lib.get_animation_list():
		var anim: Animation = _source_lib.get_animation(anim_name).duplicate(true)
		for i in anim.get_track_count():
			var path := String(anim.track_get_path(i))
			if path.begins_with(_source_skeleton_path + ":"):
				anim.track_set_path(i, NodePath(skeleton_path + path.substr(_source_skeleton_path.length())))
		lib.add_animation(anim_name, anim)
	_libs[key] = lib
	return lib


static func _load_source() -> void:
	if _source_lib != null:
		return
	_source_lib = AnimationLibrary.new()
	for src in ANIMATION_SOURCES:
		var scene: Node = (load(src) as PackedScene).instantiate()
		var ap: AnimationPlayer = scene.find_children("*", "AnimationPlayer", true, false)[0]
		var skeleton := _find_skeleton(scene)
		# Track paths are relative to the player's root node.
		var path := String(ap.get_node(ap.root_node).get_path_to(skeleton))
		if _source_skeleton_path.is_empty():
			_source_skeleton_path = path
		var lib := ap.get_animation_library(ap.get_animation_library_list()[0])
		for anim_name in lib.get_animation_list():
			if _source_lib.has_animation(anim_name):
				continue  # first source wins (both have A_TPose)
			var anim: Animation = lib.get_animation(anim_name).duplicate(true)
			if path != _source_skeleton_path:
				for i in anim.get_track_count():
					var tp := String(anim.track_get_path(i))
					if tp.begins_with(path + ":"):
						anim.track_set_path(i, NodePath(_source_skeleton_path + tp.substr(path.length())))
			_source_lib.add_animation(anim_name, anim)
		scene.free()
