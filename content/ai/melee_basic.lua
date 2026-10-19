-- Basic melee AI: wander near home, aggro on players that come close, chase
-- and attack, and run home (healing fully) when pulled past the leash.
--
-- think(npc, mem) runs 5 times per second. `mem` is a table private to this
-- NPC that survives between calls; it is cleared when the NPC respawns.

local M = {}

local function dist(x1, z1, x2, z2)
  return math.sqrt((x1 - x2) ^ 2 + (z1 - z2) ^ 2)
end

function M.think(npc, mem)
  local hx, hz = npc:home()
  local x, z = npc:position()
  local from_home = dist(x, z, hx, hz)

  if mem.returning then
    if from_home < 1.0 then
      mem.returning = false
      npc:reset()
    else
      npc:move_to(hx, hz)
      return
    end
  end

  local target = npc:target()
  if target then
    local d = npc:distance_to(target)
    if d == nil or from_home > npc:stat("leash_radius") then
      npc:set_target(nil)
      mem.returning = true
      npc:move_to(hx, hz)
      return
    end
    local range = npc:stat("attack_range")
    if d <= range then
      npc:stop()
      npc:attack(target)
    else
      npc:chase(target, range * 0.8)
    end
    return
  end

  local id = npc:nearest_player(npc:stat("aggro_radius"))
  if id then
    npc:set_target(id)
    return
  end

  -- Wander: a new point near home every 2-5 seconds.
  mem.idle = (mem.idle or 0) - 1
  if mem.idle <= 0 then
    mem.idle = math.random(10, 25)
    local a = math.random() * 2 * math.pi
    local r = math.random() * npc:stat("wander_radius")
    npc:move_to(hx + math.cos(a) * r, hz + math.sin(a) * r)
  end
end

return M
