// Command server runs the Convergence game server: account API, WebSocket
// gateway and zone simulation in one process (modular monolith).
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"convergence/server/internal/account"
	"convergence/server/internal/api"
	"convergence/server/internal/auth"
	"convergence/server/internal/content"
	"convergence/server/internal/db"
	"convergence/server/internal/fate"
	"convergence/server/internal/gateway"
	"convergence/server/internal/model"
	"convergence/server/internal/persist"
	"convergence/server/internal/zone"
)

// Character models the client knows how to render.
var playerModels = map[string]bool{
	"mannequin_m": true,
	"mannequin_f": true,
	"zuo_mo":      true,
}

const startZone = "jade_terraces"

func main() {
	loadDotEnv()
	addr := flag.String("addr", ":7777", "HTTP/WebSocket listen address")
	contentDir := flag.String("content", findDir("content"), "content directory")
	authBurst := flag.Int("auth-burst", 10, "register/login requests allowed per IP at once (raise for bot load tests)")
	var opts options
	flag.StringVar(&opts.fateModel, "fate-model", "deepseek-v4.1-flash:cloud", "Ollama model for the Fate agent; empty disables the agent (fallback cards only)")
	flag.StringVar(&opts.fateURL, "fate-url", "http://127.0.0.1:11434", "Ollama base URL")
	flag.DurationVar(&opts.fateTimeout, "fate-timeout", 30*time.Second, "time limit for one Fate deal")
	flag.BoolVar(&opts.dev, "dev", false, "development mode: reload content (YAML and Lua) when files change")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(*addr, *contentDir, *authBurst, opts, log); err != nil {
		log.Error("server failed", "err", err)
		os.Exit(1)
	}
}

type options struct {
	fateModel   string
	fateURL     string
	fateTimeout time.Duration
	dev         bool
}

func run(addr, contentDir string, authBurst int, opts options, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set (see .env)")
	}
	if err := db.Migrate(ctx, dbURL); err != nil {
		return err
	}
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	accounts := account.NewService(pool)

	c, err := content.Load(contentDir)
	if err != nil {
		return err
	}
	zc, ok := c.Zones[startZone]
	if !ok {
		return errors.New("content has no zone " + startZone)
	}

	saver := persist.NewSaver(accounts, log, time.Second)
	defer saver.Close() // runs after the zone has saved everyone

	zcfg := zone.ConfigFor(zc)
	if err := zone.LoadTerrain(&zcfg, zc, contentDir); err != nil {
		return err
	}
	z, err := zone.New(zcfg, c, zc, saver, log)
	if err != nil {
		return err
	}
	if c.Fate != nil {
		var oracle fate.Oracle
		if opts.fateModel != "" {
			oracle = &fate.Ollama{URL: opts.fateURL, Model: opts.fateModel}
			log.Info("fate agent", "model", opts.fateModel, "url", opts.fateURL)
		} else {
			log.Info("fate agent disabled; using fallback cards")
		}
		director := fate.NewDirector(oracle, 4, opts.fateTimeout, log)
		defer director.Close() // after the zone stops (defers run in reverse)
		z.SetFate(director)
	}
	go z.Run(ctx)
	if opts.dev {
		go zone.WatchContent(ctx, contentDir, z, log)
		log.Info("dev mode: watching content for changes", "dir", contentDir)
	}
	defer func() { <-z.Done() }()

	tickets := auth.NewStore[model.Character](60 * time.Second)
	mux := http.NewServeMux()
	(&api.Server{
		Accounts:  accounts,
		Sessions:  auth.NewStore[int64](time.Hour),
		Tickets:   tickets,
		Models:    playerModels,
		Classes:   classHP(c),
		Start:     api.NewCharacter{ZoneID: zc.ID, Spawn: z.Config().Spawn},
		WSPath:    "/ws",
		Log:       log,
		AuthBurst: authBurst,
	}).Routes(mux)
	mux.Handle("/ws", gateway.New(tickets, z, log))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Info("listening", "addr", addr, "content", contentDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// classHP lists the playable classes with their level-1 health.
func classHP(c *content.Content) map[string]int {
	out := map[string]int{}
	for key, cl := range c.Classes {
		out[key] = cl.MaxHP(1)
	}
	return out
}

// findDir looks for name in the working directory and its parents, so the
// server works when started from the repo root or from server/.
func findDir(name string) string {
	dir, _ := os.Getwd()
	for range 3 {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		dir = filepath.Dir(dir)
	}
	return name
}

// loadDotEnv sets variables from the nearest .env file that are not already
// set in the environment.
func loadDotEnv() {
	dir, _ := os.Getwd()
	for range 3 {
		f, err := os.Open(filepath.Join(dir, ".env"))
		if err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF"))
				k, v, ok := strings.Cut(line, "=")
				if !ok || strings.HasPrefix(line, "#") {
					continue
				}
				if _, set := os.LookupEnv(k); !set {
					os.Setenv(k, v)
				}
			}
			return
		}
		dir = filepath.Dir(dir)
	}
}
