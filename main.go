package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

func staticStat(dir string) (fs.FileInfo, error) { return os.Stat(dir) }

func main() {
	cfg := loadConfig()

	store := newStore(cfg.DataDir)
	gw := newGateway(cfg)
	sched := newScheduler(store)
	pipe := newPipeline(cfg, store, gw, sched)
	api := newAPI(cfg, store, gw, pipe, sched)

	// Nightly rollover job: in production this is a cron; here it's a ticker.
	// It sweeps every user, rolling missed sessions forward and shifting finish dates.
	go func() {
		interval := time.Duration(cfg.RolloverIntervalMin) * time.Minute
		if interval <= 0 {
			return
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			for id := range snapshotUserIDs(store) {
				_ = api.sched.Rollover(id, today())
			}
		}
	}()

	mode := "MOCK (no OPENAI_API_KEY — realistic canned plans)"
	if gw.Enabled() {
		mode = "LIVE ChatGPT API (" + cfg.ModelFast + " / " + cfg.ModelSmart + ")"
	}
	log.Printf("start.ai backend listening on :%s", cfg.Port)
	log.Printf("AI mode: %s", mode)
	if cfg.FrontendDir != "" {
		if fi, err := staticStat(cfg.FrontendDir); err == nil && fi.IsDir() {
			log.Printf("serving frontend from %s → open http://localhost:%s", cfg.FrontendDir, cfg.Port)
		}
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

func snapshotUserIDs(store *Store) map[string]struct{} {
	store.mu.RLock()
	defer store.mu.RUnlock()
	out := make(map[string]struct{}, len(store.Users))
	for id := range store.Users {
		out[id] = struct{}{}
	}
	return out
}
