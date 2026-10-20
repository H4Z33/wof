-- Elder: a camp leader. Rallies nearby allies when a fight starts, weakens
-- its target every few hits, and palm-strikes everyone around it now and
-- then; enraged when badly hurt.
local base = lib("base")
local M = {}

function M.think(npc, mem)
  local target = base.engage(npc, mem)
  if not target then
    return
  end
  if not mem.rallied then
    mem.rallied = true
    npc:call_allies(npc:stat("rally_radius"))
  end
  local cur, max = npc:hp()
  if not mem.enraged and cur / max < npc:stat("enrage_below") then
    mem.enraged = true
    npc:self_effect("vigor", 40, 1.4)
  end
  local range = npc:stat("attack_range")
  if npc:distance_to(target) > range then
    npc:chase(target, range * 0.8)
    return
  end
  npc:stop()
  mem.swings = mem.swings or 0
  local hit
  if mem.swings > 0 and mem.swings % npc:stat("slam_every") == 0 then
    hit = npc:attack_area(npc:stat("slam_radius"))
  else
    hit = npc:attack(target)
  end
  if hit then
    mem.swings = mem.swings + 1
    if mem.swings % npc:stat("weaken_every") == 0 then
      npc:effect_on(target, "weakened", 6, 1.2)
    end
  end
end

return M
