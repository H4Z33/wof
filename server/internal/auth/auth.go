// Package auth issues random tokens with an expiry: account sessions for the
// HTTP API and single-use play tickets the client presents to the gateway in
// its Hello message.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type item[T any] struct {
	value   T
	expires time.Time
}

// Store maps random tokens to values in memory.
type Store[T any] struct {
	mu    sync.Mutex
	items map[string]item[T]
	ttl   time.Duration
	now   func() time.Time
}

func NewStore[T any](ttl time.Duration) *Store[T] {
	return &Store[T]{items: make(map[string]item[T]), ttl: ttl, now: time.Now}
}

// Issue returns a new random token for v.
func (s *Store[T]) Issue(v T) (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, it := range s.items {
		if now.After(it.expires) {
			delete(s.items, k)
		}
	}
	s.items[tok] = item[T]{value: v, expires: now.Add(s.ttl)}
	return tok, nil
}

// Redeem consumes a single-use token.
func (s *Store[T]) Redeem(tok string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[tok]
	delete(s.items, tok)
	if !ok || s.now().After(it.expires) {
		var zero T
		return zero, false
	}
	return it.value, true
}

// Get looks up a reusable token and extends its expiry (sliding session).
func (s *Store[T]) Get(tok string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[tok]
	now := s.now()
	if !ok || now.After(it.expires) {
		delete(s.items, tok)
		var zero T
		return zero, false
	}
	it.expires = now.Add(s.ttl)
	s.items[tok] = it
	return it.value, true
}
