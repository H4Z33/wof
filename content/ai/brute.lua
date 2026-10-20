-- Brute: slow and heavy. Every few attacks it slams everyone around it, and
-- below a health threshold it becomes enraged (hits harder).
local base = lib("base")
local M = {}

function M.think(npc, mem)
  local target = base.engage(npc, mem)
  if not target then
    return
  end
  local cur, max = npc:hp()
  if not mem.enraged and cur / max < npc:stat("enrage_below") then
    mem.enraged = true
    npc:self_effect("vigor", 30, 1.5)
  end
  local range = npc:stat("attack_range")
  if npc:distance_to(target) <= range then
    npc:stop()
    mem.swings = mem.swings or 0
    local slam = mem.swings > 0 and mem.swings % npc:stat("slam_every") == 0
    local hit
    if slam then
      hit = npc:attack_area(npc:stat("slam_radius"))
    else
      hit = npc:attack(target)
    end
    if hit then
      mem.swings = mem.swings + 1
    end
  else
    npc:chase(target, range * 0.8)
  end
end

return M
