## Character model registry. All models share the Quaternius UAL skeleton, so
## one animation library (from UAL1) drives every model; tracks are re-pointed
## to each model's own Skeleton3D path once and cached.
extends RefCounted

const ANIMATION_SOURCE := "res://assets/characters/mannequin_m/UAL1_Standard.glb"

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
}

const DEFAULT_MODEL := "mannequin_m"

static var _source_lib: AnimationLibrary
static var _source_skeleton_path := ""
static var _libs := {}  # model key -> AnimationLibrary retargeted for that model


static func keys() -> Array:
	return MODELS.keys()


static func label(key: String) -> String:
	return MODELS.get(key, MODELS[DEFAULT_MODEL])["label"]


## Instantiates the model and an AnimationPlayer wired to its skeleton.
## Returns [model_root: Node3D, player: AnimationPlayer].
static func instantiate(key: String) -> Array:
	if not MODELS.has(key):
		key = DEFAULT_MODEL
	var info: Dictionary = MODELS[key]
	var root: Node3D = (load(info["scene"]) as PackedScene).instantiate()

	# Drop any player the glTF brought along; we add our own below.
	for ap in root.find_children("*", "AnimationPlayer", true, false):
		ap.get_parent().remove_child(ap)
		ap.queue_free()

	if info.has("albedo"):
		var mat := StandardMaterial3D.new()
		mat.albedo_color = info["albedo"]
		mat.roughness = 0.85
		for mi in root.find_children("*", "MeshInstance3D", true, false):
			(mi as MeshInstance3D).material_override = mat

	var skeleton := _find_skeleton(root)
	var player := AnimationPlayer.new()
	root.add_child(player)
	player.root_node = NodePath("..")
	if skeleton != null:
		player.add_animation_library("", _library_for(key, String(root.get_path_to(skeleton))))
	return [root, player]


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
	var scene: Node = (load(ANIMATION_SOURCE) as PackedScene).instantiate()
	var ap: AnimationPlayer = scene.find_children("*", "AnimationPlayer", true, false)[0]
	var skeleton := _find_skeleton(scene)
	# Track paths are relative to the player's root node.
	var anim_root := ap.get_node(ap.root_node)
	_source_skeleton_path = String(anim_root.get_path_to(skeleton))
	_source_lib = ap.get_animation_library(ap.get_animation_library_list()[0])
	scene.free()
