// SPDX-License-Identifier: Apache-2.0

// Package storage persists each report's screenshots and a small metadata file on a volume,
// serves the screenshots over HTTP (so a created GitHub issue can embed them by URL), and
// lets the reconciler enumerate and delete reports once their issue is closed. Keeping the
// cleanup state on the volume (not in memory) means it survives restarts.
package storage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"oblikovati.org/reporting/internal/report"
)

// File names within a report directory.
const (
	windowFile     = "window.png"
	viewportFile   = "viewport.png"
	metaFile       = "issue.json"
	deadLetterFile = "deadletter.json"
)

// IssueMeta records which GitHub issue a stored report became, so the reconciler can poll
// its state and delete the screenshots when it closes.
type IssueMeta struct {
	Number    int       `json:"number"`
	CreatedAt time.Time `json:"createdAt"`
}

// Ref pairs a report id with its issue metadata, returned when enumerating stored reports.
type Ref struct {
	ID    string
	Issue IssueMeta
}

// DeadLetter is a report whose GitHub issue could not be created after retries: the full
// payload plus the failure cause, kept so the report is recoverable rather than lost.
type DeadLetter struct {
	Payload  report.Payload `json:"payload"`
	Error    string         `json:"error"`
	FailedAt time.Time      `json:"failedAt"`
}

// Store is a directory of per-report subdirectories.
type Store struct {
	dir string
}

// New ensures dir exists and returns a store rooted at it.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create %q: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// SaveScreenshots writes a report's two PNGs under <dir>/<id>/. Empty inputs are skipped so
// a report missing a capture still stores what it has.
func (s *Store) SaveScreenshots(id string, window, viewport []byte) error {
	reportDir := filepath.Join(s.dir, id)
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return fmt.Errorf("storage: create report dir for %q: %w", id, err)
	}
	if err := writeIfPresent(filepath.Join(reportDir, windowFile), window); err != nil {
		return err
	}
	return writeIfPresent(filepath.Join(reportDir, viewportFile), viewport)
}

// SaveIssueMeta records the GitHub issue a report became.
func (s *Store) SaveIssueMeta(id string, m IssueMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("storage: marshal issue meta for %q: %w", id, err)
	}
	path := filepath.Join(s.dir, id, metaFile)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("storage: write issue meta for %q: %w", id, err)
	}
	return nil
}

// SaveDeadLetter persists a report that failed to become a GitHub issue after retries, so
// an operator can inspect or manually recreate it instead of it being silently lost.
func (s *Store) SaveDeadLetter(id string, p report.Payload, cause error) error {
	dl := DeadLetter{Payload: p, Error: cause.Error(), FailedAt: time.Now().UTC()}
	b, err := json.Marshal(dl)
	if err != nil {
		return fmt.Errorf("storage: marshal dead letter for %q: %w", id, err)
	}
	reportDir := filepath.Join(s.dir, id)
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return fmt.Errorf("storage: create report dir for %q: %w", id, err)
	}
	if err := os.WriteFile(filepath.Join(reportDir, deadLetterFile), b, 0o644); err != nil {
		return fmt.Errorf("storage: write dead letter for %q: %w", id, err)
	}
	return nil
}

// ReadDeadLetter loads a dead-lettered report's persisted payload and failure cause, or an
// error when none was recorded for id.
func (s *Store) ReadDeadLetter(id string) (DeadLetter, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, id, deadLetterFile))
	if err != nil {
		return DeadLetter{}, fmt.Errorf("storage: read dead letter for %q: %w", id, err)
	}
	var dl DeadLetter
	if err := json.Unmarshal(b, &dl); err != nil {
		return DeadLetter{}, fmt.Errorf("storage: parse dead letter for %q: %w", id, err)
	}
	return dl, nil
}

// Reports enumerates the stored reports that have an issue recorded (so the reconciler only
// considers reports whose issue actually exists). Reports still mid-creation are skipped.
func (s *Store) Reports() ([]Ref, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("storage: read %q: %w", s.dir, err)
	}
	var refs []Ref
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := s.readMeta(e.Name())
		if err != nil {
			continue // no issue.json yet (or unreadable): not ready to reconcile
		}
		refs = append(refs, Ref{ID: e.Name(), Issue: meta})
	}
	return refs, nil
}

// Delete removes a report's directory and all its screenshots.
func (s *Store) Delete(id string) error {
	if err := os.RemoveAll(filepath.Join(s.dir, id)); err != nil {
		return fmt.Errorf("storage: delete %q: %w", id, err)
	}
	return nil
}

// FileServer serves the stored screenshots; mount it under /r/ so <dir>/<id>/window.png is
// reachable at /r/<id>/window.png. http.FileServer cleans the path, blocking traversal.
func (s *Store) FileServer() http.Handler {
	return http.FileServer(http.Dir(s.dir))
}

func (s *Store) readMeta(id string) (IssueMeta, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, id, metaFile))
	if err != nil {
		return IssueMeta{}, err
	}
	var m IssueMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return IssueMeta{}, fmt.Errorf("storage: parse issue meta for %q: %w", id, err)
	}
	return m, nil
}

func writeIfPresent(path string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("storage: write %q: %w", path, err)
	}
	return nil
}
