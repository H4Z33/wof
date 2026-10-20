## Prints every animation available to characters.
## Run: godot --headless --path client -s res://tools/list_anims.gd
extends SceneTree

const Models := preload("res://game/models.gd")


func _init() -> void:
	var parts := Models.instantiate("mannequin_m")
	var ap: AnimationPlayer = parts[1]
	print(ap.get_animation_list())
	parts[0].free()
	quit()
