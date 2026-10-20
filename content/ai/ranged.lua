-- Ranged caster: attacks from a distance and backs away when a foe closes in.
local base = lib("base")
local M = {}

function M.think(npc, mem)
  local target = base.engage(npc, mem)
  if not target then
    return
  end
  local d = npc:distance_to(target)
  local range = npc:stat("attack_range")
  if d < npc:stat("keep_away") then
    local x, z = npc:position()
    local tx, tz = npc:position_of(target)
    local away = math.max(d, 0.1)
    npc:move_to(x + (x - tx) / away * 4, z + (z - tz) / away * 4)
    npc:attack(target) -- still fires while backing off
  elseif d > range then
    npc:chase(target, range * 0.85)
  else
    npc:stop()
    npc:attack(target)
  end
end

return M
