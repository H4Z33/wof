## Prints sample heights so they can be compared with the Go implementation.
## Run: godot --headless --path client -s res://tools/check_terrain.gd
extends SceneTree

const Terrain := preload("res://game/terrain.gd")


func _init() -> void:
	var t = Terrain.load_file("res://assets/terrain/jade_terraces.terrain")
	t.max_slope = tan(deg_to_rad(40.0))
	t.water_level = -1.0
	t.wade_depth = 0.6
	print("sha256 ", t.sha256)
	for p in [Vector2(0, 0), Vector2(12.3, -7.9), Vector2(-14, 34), Vector2(55.5, 58.25), Vector2(-33.33, 21.7)]:
		var m: Vector2 = t.move(p.x, p.y, 0.25, -0.25)
		print("h(%s,%s)=%.9f move=(%.9f,%.9f)" % [p.x, p.y, t.height(p.x, p.y), m.x, m.y])
	quit()
