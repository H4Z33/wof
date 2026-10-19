// Package persist writes character state to the database in the background.
// The zone hands over snapshots without ever waiting on the database; saves
// for the same character are coalesced so only the latest state is written.
package persist

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"convergence/server/internal/model"
)

type Store interface {
	SaveCharacters(ctx context.Context, chars []model.Character) error
}

type Saver struct {
	store    Store
	log      *slog.Logger
	interval time.Duration

	mu      sync.Mutex
	pending map[int64]model.Character

	stop chan struct{}
	done chan struct{}
}

func NewSaver(store Store, log *slog.Logger, interval time.Duration) *Saver {
	s := &Saver{
		store:    store,
		log:      log.With("component", "saver"),
		interval: interval,
		pending:  make(map[int64]model.Character),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go s.run()
	return s
}

// Save queues c; it never blocks on the database.
func (s *Saver) Save(c model.Character) {
	s.mu.Lock()
	s.pending[c.ID] = c
	s.mu.Unlock()
}

// Close writes everything still queued and stops the background loop.
func (s *Saver) Close() {
	close(s.stop)
	<-s.done
}

func (s *Saver) run() {
	defer close(s.done)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.flush()
		case <-s.stop:
			s.flush()
			return
		}
	}
}

func (s *Saver) flush() {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return
	}
	batch := make([]model.Character, 0, len(s.pending))
	for _, c := range s.pending {
		batch = append(batch, c)
	}
	s.pending = make(map[int64]model.Character)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.store.SaveCharacters(ctx, batch); err != nil {
		s.log.Error("save failed, will retry", "characters", len(batch), "err", err)
		// Requeue unless a newer save for the same character arrived meanwhile.
		s.mu.Lock()
		for _, c := range batch {
			if _, newer := s.pending[c.ID]; !newer {
				s.pending[c.ID] = c
			}
		}
		s.mu.Unlock()
	}
}
