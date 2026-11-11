// Command dpd is the DeployPanel control plane. Scaffold: it loads config,
// opens the database, applies migrations and serves a health endpoint.
// See docs/ROADMAP.md for what v0.1 adds.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/OWNER/deploypanel/internal/api"
	"github.com/OWNER/deploypanel/internal/config"
	"github.com/OWNER/deploypanel/internal/core/iam"
	"github.com/OWNER/deploypanel/internal/store"
)

var version = "0.0.0-dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("dpd", version)
		return
	}

	cfg, err := config.FromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := cfg.CheckSecretKey(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.SecretKeyFile == "" {
		log.Print("warning: DP_SECRET_KEY_FILE is not set; secrets cannot be stored yet")
	}
	if !config.IsLoopbackAddr(cfg.Listen) {
		log.Printf("warning: listening on %s, which is not loopback (DP_ALLOW_PUBLIC_LISTEN is set)", cfg.Listen)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o750); err != nil {
		log.Fatalf("database directory: %v", err)
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	applied, err := store.Migrate(ctx, db)
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if len(applied) > 0 {
		log.Printf("applied migrations: %v", applied)
	}

	svc := iam.New(iam.NewDBStore(db), cfg.LoginMaxAttempts, cfg.LoginLockoutFor).
		WithSessionLifetime(cfg.SessionLifetime)
	apiSrv := api.New(svc, &cfg)

	mux := http.NewServeMux()
	mux.Handle("/", apiSrv.Handler())

	srv := &http.Server{Addr: cfg.Listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Printf("dpd %s listening on %s", version, cfg.Listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
