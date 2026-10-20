## A zone's heightmap and walking rules: the client's copy of
## server/internal/terrain. height(), can_step() and move() must match the
## Go code exactly so movement prediction agrees with the server.
extends RefCounted

const HEADER_SIZE := 24

var width := 0
var depth := 0
var spacing := 1.0
var origin_x := 0.0
var origin_z := 0.0
var heights := PackedFloat32Array()
var sha256 := ""

var max_slope := 0.84
var water_level := -1000.0
var wade_depth := 0.0


## Loads a CVTR v1 file. Returns null (and logs) on failure.
static func load_file(path: String):
	var bytes := FileAccess.get_file_as_bytes(path)
	if bytes.size() < HEADER_SIZE or bytes.slice(0, 4).get_string_from_ascii() != "CVTR" or bytes.decode_u16(4) != 1:
		push_error("Not a terrain file: %s" % path)
		return null
	var t = load("res://game/terrain.gd").new()
	t.width = bytes.decode_u16(6)
	t.depth = bytes.decode_u16(8)
	t.spacing = bytes.decode_float(12)
	t.origin_x = bytes.decode_float(16)
	t.origin_z = bytes.decode_float(20)
	t.heights = bytes.slice(HEADER_SIZE).to_float32_array()
	if t.heights.size() != t.width * t.depth:
		push_error("Terrain file size mismatch: %s" % path)
		return null
	var ctx := HashingContext.new()
	ctx.start(HashingContext.HASH_SHA256)
	ctx.update(bytes)
	t.sha256 = ctx.finish().hex_encode()
	return t


func _at(ix: int, iz: int) -> float:
	ix = clampi(ix, 0, width - 1)
	iz = clampi(iz, 0, depth - 1)
	return heights[iz * width + ix]


## Ground height at (x, z), bilinearly interpolated (terrain.go Height).
func height(x: float, z: float) -> float:
	var fx := (x - origin_x) / spacing
	var fz := (z - origin_z) / spacing
	var ix := floori(fx)
	var iz := floori(fz)
	var tx := fx - ix
	var tz := fz - iz
	var h00 := _at(ix, iz)
	var h10 := _at(ix + 1, iz)
	var h01 := _at(ix, iz + 1)
	var h11 := _at(ix + 1, iz + 1)
	var a := h00 + (h10 - h00) * tx
	var b := h01 + (h11 - h01) * tx
	return a + (b - a) * tz


## terrain.go CanStep.
func can_step(ax: float, az: float, bx: float, bz: float) -> bool:
	var hb := height(bx, bz)
	if hb < water_level - wade_depth:
		return false
	var run := sqrt((bx - ax) * (bx - ax) + (bz - az) * (bz - az))
	if run == 0.0:
		return true
	return hb - height(ax, az) <= run * max_slope


## terrain.go Move: full step, else slide along X, else along Z, else stay.
func move(x: float, z: float, dx: float, dz: float) -> Vector2:
	if can_step(x, z, x + dx, z + dz):
		return Vector2(x + dx, z + dz)
	if dx != 0.0 and can_step(x, z, x + dx, z):
		return Vector2(x + dx, z)
	if dz != 0.0 and can_step(x, z, x, z + dz):
		return Vector2(x, z + dz)
	return Vector2(x, z)


## Builds the render mesh: one vertex per sample, normals from the heights.
func build_mesh() -> ArrayMesh:
	var verts := PackedVector3Array()
	var normals := PackedVector3Array()
	var indices := PackedInt32Array()
	verts.resize(width * depth)
	normals.resize(width * depth)
	for iz in depth:
		for ix in width:
			var i := iz * width + ix
			verts[i] = Vector3(origin_x + ix * spacing, heights[i], origin_z + iz * spacing)
			var dx := _at(ix + 1, iz) - _at(ix - 1, iz)
			var dz := _at(ix, iz + 1) - _at(ix, iz - 1)
			normals[i] = Vector3(-dx, 2.0 * spacing, -dz).normalized()
	indices.resize((width - 1) * (depth - 1) * 6)
	var k := 0
	for iz in depth - 1:
		for ix in width - 1:
			var i := iz * width + ix
			indices[k] = i
			indices[k + 1] = i + 1
			indices[k + 2] = i + width
			indices[k + 3] = i + 1
			indices[k + 4] = i + width + 1
			indices[k + 5] = i + width
			k += 6
	var arrays := []
	arrays.resize(Mesh.ARRAY_MAX)
	arrays[Mesh.ARRAY_VERTEX] = verts
	arrays[Mesh.ARRAY_NORMAL] = normals
	arrays[Mesh.ARRAY_INDEX] = indices
	var mesh := ArrayMesh.new()
	mesh.add_surface_from_arrays(Mesh.PRIMITIVE_TRIANGLES, arrays)
	return mesh


func extent() -> float:
	return (width - 1) * spacing
