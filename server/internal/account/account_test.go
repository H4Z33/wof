package account

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"convergence/server/internal/db"
	"convergence/server/internal/model"
)

// testURL reads TEST_DATABASE_URL from the environment or the repo's .env.
func testURL(t *testing.T) string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	f, err := os.Open(filepath.Join("..", "..", "..", ".env"))
	if err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "TEST_DATABASE_URL="); ok {
				return v
			}
		}
	}
	t.Skip("TEST_DATABASE_URL not set; skipping database tests")
	return ""
}

func newService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	url := testURL(t)
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE accounts, characters RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return NewService(pool)
}

func TestRegisterAndAuthenticate(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if err := s.Register(ctx, "Alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(ctx, "alice", "another one"); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("duplicate (case-insensitive) register: %v", err)
	}
	if _, err := s.Authenticate(ctx, "ALICE", "correct horse"); err != nil {
		t.Errorf("login: %v", err)
	}
	if _, err := s.Authenticate(ctx, "alice", "wrong password"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("wrong password: %v", err)
	}
	if _, err := s.Authenticate(ctx, "nobody", "whatever1"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("unknown user: %v", err)
	}
}

func TestCharactersLimitNamesAndSave(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if err := s.Register(ctx, "bob", "password123"); err != nil {
		t.Fatal(err)
	}
	acct, _ := s.Authenticate(ctx, "bob", "password123")
	newChar := func(name string) (model.Character, error) {
		return s.CreateCharacter(ctx, model.Character{AccountID: acct, Name: name, Model: "zuo_mo", ZoneID: "z", Level: 1, HP: 100})
	}
	first, err := newChar("Lin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newChar("lin"); !errors.Is(err, ErrNameTaken) {
		t.Errorf("duplicate name: %v", err)
	}
	for _, n := range []string{"Bao", "Mei", "Shu"} {
		if _, err := newChar(n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := newChar("Extra"); !errors.Is(err, ErrTooManyCharacters) {
		t.Errorf("fifth character: %v", err)
	}

	first.Pos = [3]float64{3, 0, -4}
	first.Level, first.Exp, first.HP = 4, 55, 77
	if err := s.SaveCharacters(ctx, []model.Character{first}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Character(ctx, acct, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pos != first.Pos || got.Level != 4 || got.Exp != 55 || got.HP != 77 {
		t.Errorf("loaded %+v", got)
	}
	if _, err := s.Character(ctx, acct+1, first.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("other account's character: %v", err)
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := hashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := verifyPassword("s3cret", h); !ok {
		t.Error("correct password rejected")
	}
	if ok, _ := verifyPassword("s3creT", h); ok {
		t.Error("wrong password accepted")
	}
	if _, err := verifyPassword("x", "$bcrypt$nope"); err == nil {
		t.Error("malformed hash accepted")
	}
}
