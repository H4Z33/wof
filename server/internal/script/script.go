// Package script runs NPC behavior written in Lua.
//
// One Engine (one Lua VM) per zone, used only from the zone goroutine. Each
// AI script is a module returning a table with think(npc, mem). The VM is
// sandboxed (no io, os, file loading or debug) and every tick runs under a
// time budget, so a broken script cannot stall the zone.
package script

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// NPC is what scripts can see and do. The zone implements it.
type NPC interface {
	Position() (x, z float64)
	Home() (x, z float64)
	HP() (cur, max int)
	Target() uint32
	SetTarget(id uint32)
	// NearestPlayer returns the closest living player within radius.
	NearestPlayer(radius float64) (id uint32, dist float64, ok bool)
	// DistanceTo fails if the entity is gone or dead.
	DistanceTo(id uint32) (float64, bool)
	MoveTo(x, z float64)
	Chase(id uint32, stopDistance float64)
	Stop()
	// Attack performs the NPC's attack if it is off cooldown and in range.
	Attack(id uint32) bool
	Stat(name string) float64
	// Reset heals fully and clears the target (used after leashing).
	Reset()
	// PositionOf fails if the entity is gone.
	PositionOf(id uint32) (x, z float64, ok bool)
	// SelfEffect / EffectOn apply a timed effect (see content.SkillEffects).
	SelfEffect(name string, durationS, value float64) bool
	EffectOn(id uint32, name string, durationS, value float64) bool
	// AttackArea hits every player within radius (uses the attack cooldown).
	AttackArea(radius float64) bool
	// CallAllies makes idle NPCs within radius join the NPC's fight.
	CallAllies(radius float64) int
}

const npcType = "npc"

type Engine struct {
	L       *lua.LState
	log     *slog.Logger
	modules map[string]*lua.LTable
	libs    map[string]*lua.LTable // shared code for scripts, via lib(name)
	cancel  context.CancelFunc
}

// LoadLib compiles a shared library that scripts get with lib(name). Load
// libraries before the scripts that use them.
func (e *Engine) LoadLib(name, source string) error {
	fn, err := e.L.Load(strings.NewReader(source), "lib/"+name)
	if err != nil {
		return fmt.Errorf("lib %s: %w", name, err)
	}
	e.L.Push(fn)
	if err := e.L.PCall(0, 1, nil); err != nil {
		return fmt.Errorf("lib %s: %w", name, err)
	}
	t, ok := e.L.Get(-1).(*lua.LTable)
	e.L.Pop(1)
	if !ok {
		return fmt.Errorf("lib %s: must return a table", name)
	}
	e.libs[name] = t
	return nil
}

func New(log *slog.Logger) *Engine {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	for _, lib := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(lib.fn))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}
	for _, name := range []string{"dofile", "loadfile", "load", "loadstring", "require", "module", "getfenv", "setfenv", "collectgarbage"} {
		L.SetGlobal(name, lua.LNil)
	}
	e := &Engine{L: L, log: log.With("component", "lua"), modules: map[string]*lua.LTable{}, libs: map[string]*lua.LTable{}}
	L.SetGlobal("lib", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		t, ok := e.libs[name]
		if !ok {
			L.RaiseError("unknown library %q", name)
		}
		L.Push(t)
		return 1
	}))
	L.SetGlobal("print", L.NewFunction(func(L *lua.LState) int {
		parts := make([]string, L.GetTop())
		for i := range parts {
			parts[i] = L.ToStringMeta(L.Get(i + 1)).String()
		}
		e.log.Info("script print", "msg", strings.Join(parts, " "))
		return 0
	}))
	L.DoString(fmt.Sprintf("math.randomseed(%d)", time.Now().UnixNano()%1_000_000_007))

	mt := L.NewTypeMetatable(npcType)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), npcMethods))
	return e
}

func (e *Engine) Close() { e.L.Close() }

// Load compiles a module and checks that it exposes think().
func (e *Engine) Load(name, source string) error {
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
	if _, ok := mod.RawGetString("think").(*lua.LFunction); !ok {
		return fmt.Errorf("%s: module has no think function", name)
	}
	e.modules[name] = mod
	return nil
}

// Brain binds one NPC to a loaded module.
type Brain struct {
	// The module is looked up on every call, so a reloaded script takes
	// effect for NPCs that already exist.
	module string
	self   *lua.LUserData
	mem    *lua.LTable
	Fails  int
}

func (e *Engine) NewBrain(module string, npc NPC) (*Brain, error) {
	if _, ok := e.modules[module]; !ok {
		return nil, fmt.Errorf("unknown AI module %q", module)
	}
	ud := e.L.NewUserData()
	ud.Value = npc
	e.L.SetMetatable(ud, e.L.GetTypeMetatable(npcType))
	return &Brain{module: module, self: ud, mem: e.L.NewTable()}, nil
}

// ResetMemory clears the brain's private table (e.g. on respawn).
func (e *Engine) ResetMemory(b *Brain) { b.mem = e.L.NewTable() }

var ErrBudget = errors.New("lua tick budget exceeded")

// BeginTick starts the time budget shared by all Think calls until EndTick.
func (e *Engine) BeginTick(budget time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	e.cancel = cancel
	e.L.SetContext(ctx)
}

func (e *Engine) EndTick() {
	e.L.RemoveContext()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
}

func (e *Engine) Think(b *Brain) error {
	if ctx := e.L.Context(); ctx != nil && ctx.Err() != nil {
		return ErrBudget
	}
	mod, ok := e.modules[b.module]
	if !ok {
		return fmt.Errorf("unknown AI module %q", b.module)
	}
	err := e.L.CallByParam(lua.P{Fn: mod.RawGetString("think"), NRet: 0, Protect: true}, b.self, b.mem)
	if err != nil {
		b.Fails++
		if ctx := e.L.Context(); ctx != nil && ctx.Err() != nil {
			return ErrBudget
		}
	}
	return err
}

func check(L *lua.LState) NPC {
	ud := L.CheckUserData(1)
	n, ok := ud.Value.(NPC)
	if !ok {
		L.ArgError(1, "npc expected")
	}
	return n
}

func optID(L *lua.LState, n int) uint32 {
	v := L.Get(n)
	if v == lua.LNil {
		return 0
	}
	return uint32(L.CheckNumber(n))
}

func pushID(L *lua.LState, id uint32) {
	if id == 0 {
		L.Push(lua.LNil)
	} else {
		L.Push(lua.LNumber(id))
	}
}

var npcMethods = map[string]lua.LGFunction{
	"position": func(L *lua.LState) int {
		x, z := check(L).Position()
		L.Push(lua.LNumber(x))
		L.Push(lua.LNumber(z))
		return 2
	},
	"home": func(L *lua.LState) int {
		x, z := check(L).Home()
		L.Push(lua.LNumber(x))
		L.Push(lua.LNumber(z))
		return 2
	},
	"hp": func(L *lua.LState) int {
		cur, max := check(L).HP()
		L.Push(lua.LNumber(cur))
		L.Push(lua.LNumber(max))
		return 2
	},
	"target": func(L *lua.LState) int {
		pushID(L, check(L).Target())
		return 1
	},
	"set_target": func(L *lua.LState) int {
		check(L).SetTarget(optID(L, 2))
		return 0
	},
	"nearest_player": func(L *lua.LState) int {
		id, d, ok := check(L).NearestPlayer(float64(L.CheckNumber(2)))
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LNumber(id))
		L.Push(lua.LNumber(d))
		return 2
	},
	"distance_to": func(L *lua.LState) int {
		d, ok := check(L).DistanceTo(optID(L, 2))
		if !ok {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(d))
		}
		return 1
	},
	"move_to": func(L *lua.LState) int {
		check(L).MoveTo(float64(L.CheckNumber(2)), float64(L.CheckNumber(3)))
		return 0
	},
	"chase": func(L *lua.LState) int {
		check(L).Chase(optID(L, 2), float64(L.OptNumber(3, 1)))
		return 0
	},
	"stop": func(L *lua.LState) int {
		check(L).Stop()
		return 0
	},
	"attack": func(L *lua.LState) int {
		L.Push(lua.LBool(check(L).Attack(optID(L, 2))))
		return 1
	},
	"stat": func(L *lua.LState) int {
		L.Push(lua.LNumber(check(L).Stat(L.CheckString(2))))
		return 1
	},
	"reset": func(L *lua.LState) int {
		check(L).Reset()
		return 0
	},
	"position_of": func(L *lua.LState) int {
		x, z, ok := check(L).PositionOf(optID(L, 2))
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LNumber(x))
		L.Push(lua.LNumber(z))
		return 2
	},
	"self_effect": func(L *lua.LState) int {
		n := check(L)
		L.Push(lua.LBool(n.SelfEffect(L.CheckString(2), float64(L.CheckNumber(3)), float64(L.OptNumber(4, 1)))))
		return 1
	},
	"effect_on": func(L *lua.LState) int {
		n := check(L)
		L.Push(lua.LBool(n.EffectOn(optID(L, 2), L.CheckString(3), float64(L.CheckNumber(4)), float64(L.OptNumber(5, 1)))))
		return 1
	},
	"attack_area": func(L *lua.LState) int {
		L.Push(lua.LBool(check(L).AttackArea(float64(L.CheckNumber(2)))))
		return 1
	},
	"call_allies": func(L *lua.LState) int {
		L.Push(lua.LNumber(check(L).CallAllies(float64(L.CheckNumber(2)))))
		return 1
	},
}
