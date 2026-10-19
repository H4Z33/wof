// Command server runs the Convergence game server: auth API, WebSocket
// gateway and zone simulation in one process (modular monolith).
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"convergence/server/internal/auth"
	"convergence/server/internal/gateway"
	"convergence/server/internal/zone"
)

// Character models the client knows how to render. Moves to content data in M2.
var models = map[string]bool{
	"mannequin_m": true,
	"mannequin_f": true,
	"zuo_mo":      true,
}

func main() {
	addr := flag.String("addr", ":7777", "HTTP/WebSocket listen address")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	tickets := auth.NewStore(60 * time.Second)
	z := zone.New(zone.DefaultConfig(), log)
	go z.Run(ctx)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/dev-login", auth.DevLoginHandler(tickets, models, "/ws"))
	mux.Handle("/ws", gateway.New(tickets, z, log))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Info("listening", "addr", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server failed", "err", err)
		os.Exit(1)
	}
}
