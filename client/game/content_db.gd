## Read-only game data from the content bundle that server/cmd/contentc
## writes to res://gen/content.json (classes, skills, item names).
extends RefCounted

const PATH := "res://gen/content.json"

static var _data := {}


static func data() -> Dictionary:
	if _data.is_empty():
		var text := FileAccess.get_file_as_string(PATH)
		var parsed = JSON.parse_string(text)
		if typeof(parsed) != TYPE_DICTIONARY:
			push_error("Content bundle missing or invalid; run tools/gen.ps1")
			parsed = {"classes": [], "items": {}}
		_data = parsed
	return _data


static func classes() -> Array:
	return data().get("classes", [])


static func class_by_key(key: String) -> Dictionary:
	for c in classes():
		if c["key"] == key:
			return c
	return {}


static func class_label(key: String) -> String:
	var c := class_by_key(key)
	return "%s (%s)" % [c["name"], c["world_name"]] if not c.is_empty() else key
