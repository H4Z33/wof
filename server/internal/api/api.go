// Package api serves the HTTP account API: register, login, character list
// and creation, and the play ticket the client uses to join the world.
package api

import (
	"cmp"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"convergence/server/internal/account"
	"convergence/server/internal/auth"
	"convergence/server/internal/model"
	"convergence/server/internal/rules"
)

// NewCharacter describes where new characters start.
type NewCharacter struct {
	ZoneID string
	Spawn  [3]float64
}

type Server struct {
	Accounts *account.Service
	Sessions *auth.Store[int64]
	Tickets  *auth.Store[model.Character]
	Models   map[string]bool
	// Classes a new character may pick (key -> max HP at level 1).
	Classes map[string]int
	Start   NewCharacter
	WSPath  string
	Log     *slog.Logger
	// AuthBurst is how many register/login requests one IP may make at once
	// (refilling at one per second). Zero means 10.
	AuthBurst int

	limMu    sync.Mutex
	limiters map[string]*rate.Limiter
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/register", s.limited(s.register))
	mux.HandleFunc("POST /api/v1/login", s.limited(s.login))
	mux.HandleFunc("GET /api/v1/characters", s.authed(s.listCharacters))
	mux.HandleFunc("POST /api/v1/characters", s.authed(s.createCharacter))
	mux.HandleFunc("POST /api/v1/play", s.authed(s.play))
}

var (
	validUsername = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)
	validCharName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{2,15}$`)
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type characterJSON struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Model  string `json:"model"`
	Level  int    `json:"level"`
	ZoneID string `json:"zone_id"`
	Luck   int    `json:"luck"`
	Karma  int    `json:"karma"`
	Class  string `json:"class"`
}

func toJSON(cs []model.Character) []characterJSON {
	out := make([]characterJSON, 0, len(cs))
	for _, c := range cs {
		out = append(out, characterJSON{ID: c.ID, Name: c.Name, Model: c.Model, Level: c.Level, ZoneID: c.ZoneID, Luck: c.Luck, Karma: c.Karma, Class: c.Class})
	}
	return out
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if !decode(w, r, &req) {
		return
	}
	if !validUsername.MatchString(req.Username) {
		fail(w, http.StatusBadRequest, "Username must be 3-20 letters, digits or _.")
		return
	}
	// Upper bound keeps hashing cost predictable.
	if len(req.Password) < 8 || len(req.Password) > 128 {
		fail(w, http.StatusBadRequest, "Password must be 8-128 characters.")
		return
	}
	switch err := s.Accounts.Register(r.Context(), req.Username, req.Password); {
	case errors.Is(err, account.ErrUsernameTaken):
		fail(w, http.StatusConflict, "That username is taken.")
	case err != nil:
		s.internal(w, "register", err)
	default:
		reply(w, http.StatusCreated, map[string]any{})
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if !decode(w, r, &req) {
		return
	}
	id, err := s.Accounts.Authenticate(r.Context(), req.Username, req.Password)
	if errors.Is(err, account.ErrBadCredentials) {
		fail(w, http.StatusUnauthorized, "Wrong username or password.")
		return
	}
	if err != nil {
		s.internal(w, "login", err)
		return
	}
	chars, err := s.Accounts.Characters(r.Context(), id)
	if err != nil {
		s.internal(w, "list characters", err)
		return
	}
	tok, err := s.Sessions.Issue(id)
	if err != nil {
		s.internal(w, "session", err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"session": tok, "characters": toJSON(chars)})
}

func (s *Server) listCharacters(w http.ResponseWriter, r *http.Request, accountID int64) {
	chars, err := s.Accounts.Characters(r.Context(), accountID)
	if err != nil {
		s.internal(w, "list characters", err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"characters": toJSON(chars)})
}

func (s *Server) createCharacter(w http.ResponseWriter, r *http.Request, accountID int64) {
	var req struct {
		Name  string `json:"name"`
		Model string `json:"model"`
		Class string `json:"class"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !validCharName.MatchString(req.Name) {
		fail(w, http.StatusBadRequest, "Name must be 3-16 letters or digits, starting with a letter.")
		return
	}
	if !s.Models[req.Model] {
		fail(w, http.StatusBadRequest, "Unknown character model.")
		return
	}
	hp, ok := s.Classes[req.Class]
	if !ok {
		fail(w, http.StatusBadRequest, "Unknown class.")
		return
	}
	c, err := s.Accounts.CreateCharacter(r.Context(), model.Character{
		AccountID: accountID,
		Name:      req.Name,
		Model:     req.Model,
		Class:     req.Class,
		ZoneID:    s.Start.ZoneID,
		Pos:       s.Start.Spawn,
		Level:     1,
		HP:        hp,
		Luck:      rules.RollLuck(),
	})
	switch {
	case errors.Is(err, account.ErrNameTaken):
		fail(w, http.StatusConflict, "That name is taken.")
	case errors.Is(err, account.ErrTooManyCharacters):
		fail(w, http.StatusConflict, "Character limit reached.")
	case err != nil:
		s.internal(w, "create character", err)
	default:
		reply(w, http.StatusCreated, map[string]any{"character": toJSON([]model.Character{c})[0]})
	}
}

func (s *Server) play(w http.ResponseWriter, r *http.Request, accountID int64) {
	var req struct {
		CharacterID int64 `json:"character_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	c, err := s.Accounts.Character(r.Context(), accountID, req.CharacterID)
	if errors.Is(err, account.ErrNotFound) {
		fail(w, http.StatusNotFound, "Character not found.")
		return
	}
	if err != nil {
		s.internal(w, "load character", err)
		return
	}
	tok, err := s.Tickets.Issue(c)
	if err != nil {
		s.internal(w, "ticket", err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"ticket": tok, "ws_path": s.WSPath})
}

// authed resolves the Bearer session token to an account ID.
func (s *Server) authed(h func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		id, valid := s.Sessions.Get(tok)
		if !ok || !valid {
			fail(w, http.StatusUnauthorized, "Session expired, please log in again.")
			return
		}
		h(w, r, id)
	}
}

// limited applies a per-IP rate limit to slow down password guessing.
func (s *Server) limited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		s.limMu.Lock()
		if s.limiters == nil || len(s.limiters) > 10000 {
			s.limiters = make(map[string]*rate.Limiter)
		}
		lim, ok := s.limiters[ip]
		if !ok {
			lim = rate.NewLimiter(rate.Every(time.Second), cmp.Or(s.AuthBurst, 10))
			s.limiters[ip] = lim
		}
		s.limMu.Unlock()
		if !lim.Allow() {
			fail(w, http.StatusTooManyRequests, "Too many attempts, wait a moment.")
			return
		}
		h(w, r)
	}
}

func (s *Server) internal(w http.ResponseWriter, what string, err error) {
	s.Log.Error("api "+what, "err", err)
	fail(w, http.StatusInternalServerError, "Internal error.")
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "Malformed request.")
		return false
	}
	return true
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}
