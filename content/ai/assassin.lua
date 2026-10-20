-- Assassin: fast hit-and-run. Strikes a few times, darts away from the
-- target, then comes back for more.
local base = lib("base")
local M = {}

function M.think(npc, mem)
  local target = base.engage(npc, mem)
  if not target then
    return
  end
  mem.ticks = (mem.ticks or 0) + 1
  if mem.retreat_until and mem.ticks < mem.retreat_until then
    return -- still darting away (move_to was set when the retreat began)
  end
  mem.retreat_until = nil

  local range = npc:stat("attack_range")
  if npc:distance_to(target) > range then
    npc:chase(target, range * 0.8)
    return
  end
  npc:stop()
  if npc:attack(target) then
    mem.strikes = (mem.strikes or 0) + 1
    if mem.strikes >= npc:stat("strikes_before_retreat") then
      mem.strikes = 0
      local x, z = npc:position()
      local tx, tz = npc:position_of(target)
      local d = math.max(base.dist(x, z, tx, tz), 0.1)
      local away = npc:stat("retreat_distance")
      npc:move_to(x + (x - tx) / d * away, z + (z - tz) / d * away)
      mem.retreat_until = mem.ticks + 6 -- about a second and a half
    end
  end
end

return M
