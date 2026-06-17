// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"
)

func TestFromEnvRequiresToken(t *testing.T) {
	t.Setenv("REPORTING_GITHUB_TOKEN", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("FromEnv should fail without a GitHub token")
	}
}

func TestFromEnvAppliesDefaults(t *testing.T) {
	t.Setenv("REPORTING_GITHUB_TOKEN", "tok")
	c, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if c.Addr != ":8080" || c.GitHubOwner != "Oblikovati" || c.GitHubRepo != "Oblikovati" {
		t.Errorf("defaults wrong: %+v", c)
	}
	if c.PollInterval != 15*time.Minute {
		t.Errorf("poll interval = %v", c.PollInterval)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("REPORTING_GITHUB_TOKEN", "tok")
	t.Setenv("REPORTING_ADDR", ":9000")
	t.Setenv("REPORTING_POLL_INTERVAL", "30s")
	t.Setenv("REPORTING_QUEUE_SIZE", "10")
	c, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if c.Addr != ":9000" || c.PollInterval != 30*time.Second || c.QueueSize != 10 {
		t.Errorf("overrides not applied: %+v", c)
	}
}
