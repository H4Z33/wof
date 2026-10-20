-- Flowing Strikes: several quick hits on the target. Each hit is its own
-- damage event, so on-hit effects and numbers show one by one.
local M = {}

function M.on_cast(ctx)
  local target = ctx:target()
  for _ = 1, ctx:param("hits") do
    if not ctx:alive(target) then
      return
    end
    ctx:damage(target, ctx:roll(ctx:param("min"), ctx:param("max")))
  end
end

return M
