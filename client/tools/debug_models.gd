## Prints how each character model and its animation wiring resolve.
## Run: godot --headless --path client -s res://tools/debug_models.gd
extends SceneTree

const Models := preload("res://game/models.gd")


func _init() -> void:
	var src: Node = (load(Models.ANIMATION_SOURCE) as PackedScene).instantiate()
	_dump(src, "")
	var ap: AnimationPlayer = src.find_children("*", "AnimationPlayer", true, false)[0]
	print("source root_node=", ap.root_node, " libs=", ap.get_animation_library_list())
	var lib := ap.get_animation_library(ap.get_animation_library_list()[0])
	var a := lib.get_animation(lib.get_animation_list()[0])
	print("anims=", lib.get_animation_list().slice(0, 5), " track0=", a.track_get_path(0), " type=", a.track_get_type(0))
	src.free()
	for key in Models.keys():
		var parts := Models.instantiate(key)
		var root: Node3D = parts[0]
		var player: AnimationPlayer = parts[1]
		get_root().add_child(root)
		print("--", key)
		_dump(root, "  ")
		var anim := player.get_animation("Idle")
		print("  has Idle_Loop=", player.has_animation("Idle"), " track0=", anim.track_get_path(0) if anim else null)
		if anim:
			print("  resolves=", root.get_node_or_null(NodePath(String(anim.track_get_path(0)).split(":")[0])))
		root.queue_free()
	quit()


func _dump(n: Node, indent: String, depth := 0) -> void:
	if depth > 3:
		return
	print(indent, n.name, " <", n.get_class(), ">")
	for c in n.get_children():
		_dump(c, indent + "  ", depth + 1)
