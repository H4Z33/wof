package script

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type stubNPC struct {
	moves  [][2]float64
	target uint32
}

func (s *stubNPC) Position() (float64, float64)                  { return 0, 0 }
func (s *stubNPC) Home() (float64, float64)                      { return 0, 0 }
func (s *stubNPC) HP() (int, int)                                { return 10, 10 }
func (s *stubNPC) Target() uint32                                { return s.target }
func (s *stubNPC) SetTarget(id uint32)                           { s.target = id }
func (s *stubNPC) NearestPlayer(float64) (uint32, float64, bool) { return 7, 3, true }
func (s *stubNPC) DistanceTo(uint32) (float64, bool)             { return 3, true }
func (s *stubNPC) MoveTo(x, z float64)                           { s.moves = append(s.moves, [2]float64{x, z}) }
func (s *stubNPC) Chase(uint32, float64)                         {}
func (s *stubNPC) Stop()                                         {}
func (s *stubNPC) Attack(uint32) bool                            { return true }
func (s *stubNPC) Stat(string) float64                           { return 1 }
func (s *stubNPC) Reset()                                        {}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestThinkCallsHostAndKeepsMemory(t *testing.T) {
	e := New(quiet)
	defer e.Close()
	src := `
local M = {}
function M.think(npc, mem)
  mem.n = (mem.n or 0) + 1
  npc:move_to(mem.n, 0)
  local id = npc:nearest_player(10)
  npc:set_target(id)
end
return M`
	if err := e.Load("t.lua", src); err != nil {
		t.Fatal(err)
	}
	n := &stubNPC{}
	b, err := e.NewBrain("t.lua", n)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := e.Think(b); err != nil {
			t.Fatal(err)
		}
	}
	if len(n.moves) != 3 || n.moves[2][0] != 3 || n.target != 7 {
		t.Errorf("moves=%v target=%d", n.moves, n.target)
	}
}

func TestSandboxHasNoFileAccess(t *testing.T) {
	e := New(quiet)
	defer e.Close()
	for _, src := range []string{`return io.open("x")`, `return os.exit()`, `return dofile("x")`, `return require("x")`} {
		if err := e.Load("bad.lua", src); err == nil {
			t.Errorf("%q: expected an error", src)
		}
	}
}

func TestBudgetStopsInfiniteLoop(t *testing.T) {
	e := New(quiet)
	defer e.Close()
	if err := e.Load("loop.lua", `local M = {} function M.think() while true do end end return M`); err != nil {
		t.Fatal(err)
	}
	b, _ := e.NewBrain("loop.lua", &stubNPC{})
	start := time.Now()
	e.BeginTick(20 * time.Millisecond)
	err := e.Think(b)
	e.EndTick()
	if !errors.Is(err, ErrBudget) {
		t.Fatalf("err = %v, want ErrBudget", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("budget did not stop the script promptly")
	}
}

func TestLoadRejectsModuleWithoutThink(t *testing.T) {
	e := New(quiet)
	defer e.Close()
	if err := e.Load("x.lua", `return {}`); err == nil {
		t.Fatal("expected an error")
	}
}
