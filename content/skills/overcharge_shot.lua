-- Overcharge Shot: spends all Aether Charge; damage grows with it.
-- Returning false rejects the cast (no cooldown, nothing spent).
local M = {}

function M.on_cast(ctx)
  local charge = ctx:resource()
  if charge < ctx:param("min_charge") then
    return false, "not_enough_resource"
  end
  ctx:spend(charge)
  local mult = 1 + charge / 50
  ctx:damage(ctx:target(), ctx:roll(ctx:param("min"), ctx:param("max")) * mult)
  return true
end

return M
