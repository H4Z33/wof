"""Rig a static humanoid mesh onto the Quaternius UAL skeleton.

The result shares bone names and hierarchy with the Universal Animation
Library, so the client plays the UAL animations on it directly.

Usage (headless):
  blender -b --factory-startup --python tools/blender/rig_to_ual.py -- \
      <UAL1_Standard.glb> <input_mesh.glb> <output.glb> [--arm-angle DEG]

Steps:
  1. Scale the mesh to the mannequin's height and stand it on the ground.
  2. Rotate the T-pose upper arms down to match an arms-down mesh and apply
     that as the new rest pose. Animations keep working because they store
     absolute bone transforms.
  3. Weight with Blender's automatic (bone heat) weights; if that leaves
     vertices unweighted (common on generated, non-manifold meshes), transfer
     weights from the posed mannequin body instead.
  4. Export armature + mesh (no animations) as glTF binary.
"""

import math
import sys

import bpy
import mathutils


def parse_args():
    argv = sys.argv[sys.argv.index("--") + 1:]
    angle = None
    if "--arm-angle" in argv:
        i = argv.index("--arm-angle")
        angle = float(argv[i + 1])
        del argv[i:i + 2]
    ual, mesh, out = argv
    return ual, mesh, out, angle


def world_bbox(obj):
    pts = [obj.matrix_world @ mathutils.Vector(c) for c in obj.bound_box]
    lo = mathutils.Vector((min(p.x for p in pts), min(p.y for p in pts), min(p.z for p in pts)))
    hi = mathutils.Vector((max(p.x for p in pts), max(p.y for p in pts), max(p.z for p in pts)))
    return lo, hi


def import_glb(path):
    before = set(bpy.context.scene.objects)
    bpy.ops.import_scene.gltf(filepath=path)
    return [o for o in bpy.context.scene.objects if o not in before]


def select_only(*objs, active=None):
    bpy.ops.object.select_all(action="DESELECT")
    for o in objs:
        o.select_set(True)
    bpy.context.view_layer.objects.active = active or objs[0]


def estimate_arm_angle(mesh, arm, side):
    """Angle (deg) between the T-pose arm and the mesh's arm, seen from the front.

    side is +1 for the character's left (+X in Blender, facing -Y), -1 for right.
    """
    shoulder = arm.matrix_world @ arm.data.bones[f"upperarm_{'l' if side > 0 else 'r'}"].head_local
    verts = [mesh.matrix_world @ v.co for v in mesh.data.vertices]
    lo, hi = world_bbox(mesh)
    height = hi.z - lo.z
    # Arm candidates: outside the torso column, between hips and shoulders.
    cand = [v for v in verts
            if side * v.x > abs(shoulder.x) * 1.05
            and lo.z + 0.35 * height < v.z < shoulder.z]
    if not cand:
        return 75.0
    tip = max(cand, key=lambda v: (v - shoulder).length)
    d = tip - shoulder
    # Angle below horizontal in the XZ (front) plane.
    return math.degrees(math.atan2(-d.z, side * d.x))


def main():
    ual_path, mesh_path, out_path, forced_angle = parse_args()
    bpy.ops.wm.read_factory_settings(use_empty=True)

    ual_objs = import_glb(ual_path)
    arm = next(o for o in ual_objs if o.type == "ARMATURE")
    body = next(o for o in ual_objs if o.type == "MESH" and o.name.startswith("Mannequin"))
    for o in ual_objs:
        if o.type == "MESH" and o is not body:
            bpy.data.objects.remove(o)
    if arm.animation_data:
        arm.animation_data_clear()
    for pb in arm.pose.bones:
        pb.matrix_basis.identity()
    bpy.context.view_layer.update()

    mesh_objs = import_glb(mesh_path)
    meshes = [o for o in mesh_objs if o.type == "MESH"]
    if len(meshes) != 1:
        raise SystemExit(f"expected one mesh in {mesh_path}, found {len(meshes)}")
    mesh = meshes[0]
    # Bake any parent/empty transforms into the mesh and detach it.
    select_only(mesh)
    bpy.ops.object.parent_clear(type="CLEAR_KEEP_TRANSFORM")
    bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)
    for o in mesh_objs:
        if o is not mesh:
            bpy.data.objects.remove(o)

    # 1. Match height and stand on the ground, centered like the mannequin.
    blo, bhi = world_bbox(body)
    mlo, mhi = world_bbox(mesh)
    s = (bhi.z - blo.z) / (mhi.z - mlo.z)
    mesh.scale = (s, s, s)
    bpy.context.view_layer.update()
    mlo, mhi = world_bbox(mesh)
    mesh.location += mathutils.Vector((-(mlo.x + mhi.x) / 2, -(mlo.y + mhi.y) / 2, blo.z - mlo.z))
    bpy.context.view_layer.update()
    select_only(mesh)
    bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)

    # 2. Pose the arms down and make that the rest pose.
    angles = {}
    for side, tag in ((1, "l"), (-1, "r")):
        angles[tag] = forced_angle if forced_angle is not None else estimate_arm_angle(mesh, arm, side)
    print(f"RIG arm angles below horizontal: L={angles['l']:.1f} R={angles['r']:.1f}")

    select_only(arm)
    bpy.ops.object.mode_set(mode="POSE")
    for side, tag in ((1, "l"), (-1, "r")):
        pb = arm.pose.bones[f"upperarm_{tag}"]
        # Rotate about the world front axis (Y); left arm goes down with a
        # negative rotation, right arm with a positive one.
        rot = mathutils.Matrix.Rotation(math.radians(-side * angles[tag]), 4, "Y")
        head = pb.head.copy()
        m = pb.matrix.copy()
        m.translation -= head
        m = rot @ m
        m.translation += head
        pb.matrix = m
        bpy.context.view_layer.update()
    bpy.ops.object.mode_set(mode="OBJECT")

    # Keep a posed copy of the mannequin body for the weight-transfer fallback.
    select_only(body)
    bpy.ops.object.duplicate()
    donor = bpy.context.active_object
    for mod in list(donor.modifiers):
        if mod.type == "ARMATURE":
            bpy.ops.object.modifier_apply(modifier=mod.name)
    donor.parent = None

    select_only(arm)
    bpy.ops.object.mode_set(mode="POSE")
    bpy.ops.pose.select_all(action="SELECT")
    bpy.ops.pose.armature_apply(selected=False)
    bpy.ops.object.mode_set(mode="OBJECT")
    bpy.data.objects.remove(body)

    # 3. Weights.
    select_only(mesh, arm, active=arm)
    bpy.ops.object.parent_set(type="ARMATURE_AUTO")
    weighted = sum(1 for v in mesh.data.vertices if any(g.weight > 0 for g in v.groups))
    total = len(mesh.data.vertices)
    print(f"RIG automatic weights: {weighted}/{total} vertices weighted")
    method = "automatic"
    if weighted < 0.99 * total:
        method = "transfer"
        for vg in list(mesh.vertex_groups):
            mesh.vertex_groups.remove(vg)
        for vg in donor.vertex_groups:
            mesh.vertex_groups.new(name=vg.name)
        mod = mesh.modifiers.new("WeightTransfer", "DATA_TRANSFER")
        mod.object = donor
        mod.use_vert_data = True
        mod.data_types_verts = {"VGROUP_WEIGHTS"}
        mod.vert_mapping = "POLYINTERP_NEAREST"
        mod.layers_vgroup_select_src = "ALL"
        mod.layers_vgroup_select_dst = "NAME"
        select_only(mesh)
        # The transfer modifier must be applied before the armature modifier.
        bpy.ops.object.modifier_move_to_index(modifier=mod.name, index=0)
        bpy.ops.object.modifier_apply(modifier=mod.name)
        weighted = sum(1 for v in mesh.data.vertices if any(g.weight > 0 for g in v.groups))
        print(f"RIG transferred weights: {weighted}/{total} vertices weighted")
    bpy.data.objects.remove(donor)

    # Limit to 4 influences per vertex (glTF/Godot default) and normalize.
    select_only(mesh)
    bpy.ops.object.vertex_group_limit_total(group_select_mode="ALL", limit=4)
    bpy.ops.object.vertex_group_normalize_all(lock_active=False)

    # 4. Export.
    select_only(arm, mesh)
    bpy.ops.export_scene.gltf(
        filepath=out_path,
        export_format="GLB",
        use_selection=True,
        export_animations=False,
        export_skins=True,
        export_apply=False,
    )
    print(f"RIG done method={method} -> {out_path}")


main()
