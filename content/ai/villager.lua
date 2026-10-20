-- Villagers: wander near home and react to players nearby, like the folk
-- of Fable. Evil heroes make them flee in fear; famous, kind ones get
-- cheers. Attacked villagers run and cry for help.
local base = lib("base")
local M = {}

local lines = {
  hero = {
    "It's you! The one everyone talks about!",
    "Thank the heavens you're here.",
    "My children want to be like you.",
    "The terraces are safer because of you.",
  },
  known = {
    "I've heard of you, cultivator.",
    "Good day to you.",
    "Watch out for the bandits up the hill.",
  },
  stranger = {
    "Another wanderer...",
    "Mind the paddies.",
    "Hm? Do I know you?",
  },
  evil = {
    "Stay away from me!",
    "Please, I have a family!",
    "Monster...",
    "Hide the children!",
  },
  hurt = {
    "Help! Somebody help!",
    "Why would you do this?!",
    "Guards! Anyone!",
  },
}

local function pick(t)
  return t[math.random(#t)]
end

local function speak(npc, mem, kind)
  mem.ticks = mem.ticks or 0
  if (mem.next_line or 0) > mem.ticks then
    return
  end
  mem.next_line = mem.ticks + math.random(30, 50) -- 6-10 seconds
  npc:say(pick(lines[kind]))
end

function M.think(npc, mem)
  mem.ticks = (mem.ticks or 0) + 1
  local x, z = npc:position()

  -- Attacked: run from the attacker.
  local attacker = npc:target()
  if attacker and npc:distance_to(attacker) then
    local ax, az = npc:position_of(attacker)
    local d = math.max(base.dist(x, z, ax, az), 0.1)
    npc:move_to(x + (x - ax) / d * 6, z + (z - az) / d * 6)
    npc:emote("cower")
    speak(npc, mem, "hurt")
    if d > 15 then
      npc:set_target(nil)
    end
    return
  end

  local id, dist = npc:nearest_player(npc:stat("notice_radius"))
  if id then
    local renown, karma = npc:player_info(id)
    if karma <= -30 then
      local px, pz = npc:position_of(id)
      local d = math.max(dist, 0.1)
      npc:move_to(x + (x - px) / d * 5, z + (z - pz) / d * 5)
      npc:emote("cower")
      speak(npc, mem, "evil")
    elseif renown >= 60 and karma >= 0 then
      npc:stop()
      npc:face(id)
      npc:emote("cheer")
      speak(npc, mem, "hero")
    else
      npc:stop()
      npc:face(id)
      npc:emote("")
      speak(npc, mem, renown >= 20 and "known" or "stranger")
    end
    return
  end

  npc:emote("")
  base.wander(npc, mem)
end

return M
