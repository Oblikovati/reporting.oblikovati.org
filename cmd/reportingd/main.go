// SPDX-License-Identifier: Apache-2.0

// Command reportingd is the reporting.oblikovati.org service: it ingests CRC-authorized bug
// reports from the Oblikovati application, queues them in memory, and — in the background —
// stores their screenshots and opens a GitHub issue that embeds them. A reconciler deletes a
// report's screenshots once its issue is closed.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"oblikovati.org/reporting/internal/config"
	"oblikovati.org/reporting/internal/github"
	"oblikovati.org/reporting/internal/httpapi"
	"oblikovati.org/reporting/internal/queue"
	"oblikovati.org/reporting/internal/reconciler"
	"oblikovati.org/reporting/internal/storage"
	"oblikovati.org/reporting/internal/worker"
)

// shutdownGrace bounds how long in-flight HTTP requests get to finish on shutdown.
const shutdownGrace = 10 * time.Second

func main() {
	// -healthcheck is the container HEALTHCHECK probe: it pings the local /healthz endpoint
	// and exits 0/1, so the distroless image needs no curl/wget. It does not need the config
	// (or the GitHub token), so handle it before FromEnv.
	healthcheck := flag.Bool("healthcheck", false, "probe the local /healthz endpoint and exit")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("reportingd: %v", err)
	}

	store, err := storage.New(cfg.StorageDir)
	if err != nil {
		log.Fatalf("reportingd: %v", err)
	}
	jobs := queue.New(cfg.QueueSize)
	gh := github.New(cfg.GitHubToken, cfg.GitHubOwner, cfg.GitHubRepo, &http.Client{Timeout: 20 * time.Second})
	if cfg.GitHubAPIBase != "" {
		gh.SetAPIBase(cfg.GitHubAPIBase)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go worker.New(jobs, gh, store, cfg.PublicBaseURL).Run(ctx)
	go reconciler.New(store, gh, cfg.PollInterval).Run(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(jobs, store.FileServer(), cfg.MaxBodyBytes).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	serve(ctx, srv)
}

// serve runs the HTTP server until the context is cancelled, then drains gracefully.
func serve(ctx context.Context, srv *http.Server) {
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("reportingd: shutdown: %v", err)
		}
	}()
	log.Printf("reportingd: listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("reportingd: %v", err)
	}
	log.Printf("reportingd: stopped")
}

// runHealthcheck performs a GET against the local health endpoint, returning a process exit
// code (0 healthy, 1 not). The address mirrors REPORTING_ADDR so a custom port still works.
func runHealthcheck() int {
	addr := os.Getenv("REPORTING_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	url := fmt.Sprintf("http://127.0.0.1%s/healthz", addr)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: status %s\n", resp.Status)
		return 1
	}
	return 0
}
