-- Shared AI behavior, used with lib("base"). Handles everything around a
-- fight: walking home (and healing) after being pulled too far, picking up
-- targets, and wandering when idle.
local B = {}

function B.dist(x1, z1, x2, z2)
  return math.sqrt((x1 - x2) ^ 2 + (z1 - z2) ^ 2)
end

-- Returns the target to fight now, or nil (it is busy going home or idle).
function B.engage(npc, mem)
  local hx, hz = npc:home()
  local x, z = npc:position()
  local from_home = B.dist(x, z, hx, hz)

  if mem.returning then
    if from_home < 1.0 then
      mem.returning = false
      npc:reset()
      B.forget_fight(mem)
    else
      npc:move_to(hx, hz)
      return nil
    end
  end

  local target = npc:target()
  if target then
    if npc:distance_to(target) == nil or from_home > npc:stat("leash_radius") then
      npc:set_target(nil)
      mem.returning = true
      npc:move_to(hx, hz)
      return nil
    end
    return target
  end

  local id = npc:nearest_player(npc:stat("aggro_radius"))
  if id then
    npc:set_target(id)
    return id
  end
  B.wander(npc, mem)
  return nil
end

-- Clears per-fight memory (counters, enrage) after a reset.
function B.forget_fight(mem)
  mem.swings, mem.enraged, mem.rallied, mem.strikes, mem.retreat_until = 0, false, false, 0, nil
end

-- A new point near home every 2-5 seconds.
function B.wander(npc, mem)
  mem.idle = (mem.idle or 0) - 1
  if mem.idle > 0 then
    return
  end
  mem.idle = math.random(10, 25)
  local hx, hz = npc:home()
  local a = math.random() * 2 * math.pi
  local r = math.random() * npc:stat("wander_radius")
  npc:move_to(hx + math.cos(a) * r, hz + math.sin(a) * r)
end

return B
