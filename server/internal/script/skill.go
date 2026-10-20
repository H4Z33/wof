package script

import (
	"context"
	"fmt"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// SkillHost is what a skill script can see and do during one cast. The
// zone implements it; amounts are before the caster's and target's effect
// multipliers, which the host applies.
type SkillHost interface {
	Caster() uint32
	Target() uint32
	Alive(id uint32) bool
	// Resource is the caster's class resource (Qi, Aether Charge...).
	Resource() float64
	Spend(amount float64) bool
	Damage(id uint32, amount float64) int
	Heal(id uint32, amount float64) int
	Effect(id uint32, name string, durationS, value float64) bool
	// EnemiesNear lists living NPCs within radius of the entity id.
	EnemiesNear(id uint32, radius float64) []uint32
	Param(name string) float64
	Roll(lo, hi int) int
}

const skillType = "skillctx"

// castBudget bounds one skill script call.
const castBudget = 5 * time.Millisecond

func (e *Engine) skillMeta() *lua.LTable {
	if mt, ok := e.L.GetTypeMetatable(skillType).(*lua.LTable); ok && mt != nil {
		return mt
	}
	mt := e.L.NewTypeMetatable(skillType)
	e.L.SetField(mt, "__index", e.L.SetFuncs(e.L.NewTable(), skillMethods))
	return mt
}

// LoadSkill compiles a skill module and checks that it exposes on_cast().
func (e *Engine) LoadSkill(name, source string) error {
	fn, err := e.L.Load(strings.NewReader(source), name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	e.L.Push(fn)
	if err := e.L.PCall(0, 1, nil); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	mod, ok := e.L.Get(-1).(*lua.LTable)
	e.L.Pop(1)
	if !ok {
		return fmt.Errorf("%s: script must return a table", name)
	}
	if _, ok := mod.RawGetString("on_cast").(*lua.LFunction); !ok {
		return fmt.Errorf("%s: module has no on_cast function", name)
	}
	e.modules[name] = mod
	return nil
}

// Cast runs a skill module's on_cast. The script may return false and a
// reason to reject the cast (nothing happens, no cooldown); returning
// nothing means it went off.
func (e *Engine) Cast(name string, host SkillHost) (ok bool, reason string, err error) {
	mod, found := e.modules[name]
	if !found {
		return false, "", fmt.Errorf("unknown skill module %q", name)
	}
	ud := e.L.NewUserData()
	ud.Value = host
	e.L.SetMetatable(ud, e.skillMeta())

	// Casts happen between ticks, outside the AI budget, so they get their own.
	if e.L.Context() == nil {
		ctx, cancel := context.WithTimeout(context.Background(), castBudget)
		defer cancel()
		e.L.SetContext(ctx)
		defer e.L.RemoveContext()
	}
	if err := e.L.CallByParam(lua.P{Fn: mod.RawGetString("on_cast"), NRet: 2, Protect: true}, ud); err != nil {
		return false, "", err
	}
	ret, why := e.L.Get(-2), e.L.Get(-1)
	e.L.Pop(2)
	if ret == lua.LFalse {
		return false, lua.LVAsString(why), nil
	}
	return true, "", nil
}

func checkSkill(L *lua.LState) SkillHost {
	ud := L.CheckUserData(1)
	h, ok := ud.Value.(SkillHost)
	if !ok {
		L.ArgError(1, "skill context expected")
	}
	return h
}

var skillMethods = map[string]lua.LGFunction{
	"caster": func(L *lua.LState) int { L.Push(lua.LNumber(checkSkill(L).Caster())); return 1 },
	"target": func(L *lua.LState) int { pushID(L, checkSkill(L).Target()); return 1 },
	"alive": func(L *lua.LState) int {
		L.Push(lua.LBool(checkSkill(L).Alive(optID(L, 2))))
		return 1
	},
	"resource": func(L *lua.LState) int { L.Push(lua.LNumber(checkSkill(L).Resource())); return 1 },
	"spend": func(L *lua.LState) int {
		L.Push(lua.LBool(checkSkill(L).Spend(float64(L.CheckNumber(2)))))
		return 1
	},
	"damage": func(L *lua.LState) int {
		L.Push(lua.LNumber(checkSkill(L).Damage(optID(L, 2), float64(L.CheckNumber(3)))))
		return 1
	},
	"heal": func(L *lua.LState) int {
		L.Push(lua.LNumber(checkSkill(L).Heal(optID(L, 2), float64(L.CheckNumber(3)))))
		return 1
	},
	"effect": func(L *lua.LState) int {
		h := checkSkill(L)
		L.Push(lua.LBool(h.Effect(optID(L, 2), L.CheckString(3), float64(L.CheckNumber(4)), float64(L.OptNumber(5, 1)))))
		return 1
	},
	"enemies_near": func(L *lua.LState) int {
		ids := checkSkill(L).EnemiesNear(optID(L, 2), float64(L.CheckNumber(3)))
		t := L.NewTable()
		for _, id := range ids {
			t.Append(lua.LNumber(id))
		}
		L.Push(t)
		return 1
	},
	"param": func(L *lua.LState) int {
		L.Push(lua.LNumber(checkSkill(L).Param(L.CheckString(2))))
		return 1
	},
	"roll": func(L *lua.LState) int {
		L.Push(lua.LNumber(checkSkill(L).Roll(int(L.CheckNumber(2)), int(L.CheckNumber(3)))))
		return 1
	},
}
