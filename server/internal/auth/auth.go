// Package auth issues the short-lived tickets a client presents to the
// gateway in its Hello message.
//
// M0 has no accounts: the dev login accepts any valid name. Real accounts
// (Argon2id hashes in PostgreSQL) replace DevLoginHandler in M1; the ticket
// flow stays the same.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// Identity is who a ticket logs in as.
type Identity struct {
	Name  string
	Model string
}

type ticket struct {
	id      Identity
	expires time.Time
}

// Store holds single-use tickets in memory.
type Store struct {
	mu      sync.Mutex
	tickets map[string]ticket
	ttl     time.Duration
	now     func() time.Time
}

func NewStore(ttl time.Duration) *Store {
	return &Store{tickets: make(map[string]ticket), ttl: ttl, now: time.Now}
}

// Issue returns a new random ticket for id.
func (s *Store) Issue(id Identity) (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, t := range s.tickets {
		if now.After(t.expires) {
			delete(s.tickets, k)
		}
	}
	s.tickets[tok] = ticket{id: id, expires: now.Add(s.ttl)}
	return tok, nil
}

// Redeem consumes a ticket. Each ticket works once.
func (s *Store) Redeem(tok string) (Identity, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[tok]
	if !ok {
		return Identity{}, false
	}
	delete(s.tickets, tok)
	if s.now().After(t.expires) {
		return Identity{}, false
	}
	return t.id, true
}

var validName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{2,15}$`)

type loginRequest struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type loginResponse struct {
	Token  string `json:"token"`
	WSPath string `json:"ws_path"`
}

// DevLoginHandler serves POST {"name","model"} -> {"token","ws_path"}.
// models is the set of character models the client may pick.
func DevLoginHandler(s *Store, models map[string]bool, wsPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req loginRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !validName.MatchString(req.Name) {
			http.Error(w, "name must be 3-16 letters, digits or _, starting with a letter", http.StatusBadRequest)
			return
		}
		if !models[req.Model] {
			http.Error(w, "unknown model", http.StatusBadRequest)
			return
		}
		tok, err := s.Issue(Identity{Name: req.Name, Model: req.Model})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loginResponse{Token: tok, WSPath: wsPath})
	})
}
