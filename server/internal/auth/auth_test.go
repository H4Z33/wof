package auth

import (
	"testing"
	"time"
)

func TestRedeemIsSingleUse(t *testing.T) {
	s := NewStore[int](time.Minute)
	tok, _ := s.Issue(42)
	if v, ok := s.Redeem(tok); !ok || v != 42 {
		t.Fatalf("first redeem = %v, %v", v, ok)
	}
	if _, ok := s.Redeem(tok); ok {
		t.Fatal("second redeem succeeded")
	}
}

func TestExpiryAndSliding(t *testing.T) {
	now := time.Unix(0, 0)
	s := NewStore[string](10 * time.Second)
	s.now = func() time.Time { return now }
	tok, _ := s.Issue("acct")

	now = now.Add(8 * time.Second)
	if _, ok := s.Get(tok); !ok {
		t.Fatal("session expired too early")
	}
	now = now.Add(8 * time.Second) // 16 s after issue, 8 s after last use
	if _, ok := s.Get(tok); !ok {
		t.Fatal("Get did not extend the session")
	}
	now = now.Add(11 * time.Second)
	if _, ok := s.Get(tok); ok {
		t.Fatal("session did not expire")
	}
}
