// SPDX-License-Identifier: Apache-2.0

// Package config loads the reporting service's runtime configuration from the environment,
// applying sensible defaults so only the secrets (the GitHub token) are mandatory.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the resolved runtime configuration.
type Config struct {
	Addr          string        // listen address, e.g. ":8080"
	PublicBaseURL string        // external base URL used to build screenshot links in issues
	StorageDir    string        // directory (a mounted volume) for per-report screenshots + metadata
	GitHubToken   string        // PAT used to open and read issues (the only required secret)
	GitHubOwner   string        // issue repo owner
	GitHubRepo    string        // issue repo name
	GitHubAPIBase string        // REST API base; empty ⇒ the public api.github.com (override for staging/tests)
	PollInterval  time.Duration // how often the reconciler checks issue state to clean up images
	QueueSize     int           // in-memory queue capacity
	MaxBodyBytes  int64         // request body cap (screenshots make reports large)
}

// FromEnv resolves the configuration, returning an error only when a required secret is
// missing. Every other value has a default suitable for a single-container deployment.
func FromEnv() (Config, error) {
	c := Config{
		Addr:          env("REPORTING_ADDR", ":8080"),
		PublicBaseURL: env("REPORTING_PUBLIC_BASE_URL", "https://reporting.oblikovati.org"),
		StorageDir:    env("REPORTING_STORAGE_DIR", "/data/reports"),
		GitHubToken:   os.Getenv("REPORTING_GITHUB_TOKEN"),
		GitHubOwner:   env("REPORTING_GITHUB_OWNER", "Oblikovati"),
		GitHubRepo:    env("REPORTING_GITHUB_REPO", "Oblikovati"),
		GitHubAPIBase: os.Getenv("REPORTING_GITHUB_API_BASE"),
		PollInterval:  envDuration("REPORTING_POLL_INTERVAL", 15*time.Minute),
		QueueSize:     envInt("REPORTING_QUEUE_SIZE", 256),
		MaxBodyBytes:  int64(envInt("REPORTING_MAX_BODY_BYTES", 25<<20)),
	}
	if c.GitHubToken == "" {
		return Config{}, fmt.Errorf("config: REPORTING_GITHUB_TOKEN is required (a GitHub PAT with issues:write)")
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
