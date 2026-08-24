// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"oblikovati.org/reporting/internal/github"
	"oblikovati.org/reporting/internal/queue"
	"oblikovati.org/reporting/internal/report"
	"oblikovati.org/reporting/internal/storage"
)

// issueResult is one fakeIssuer.CreateIssue call's canned return.
type issueResult struct {
	num int
	err error
}

// fakeIssuer records the issue it was asked to create instead of calling GitHub. When
// results is set, calls consume it in order (so a test can script "fails, then succeeds");
// past the end of results (or when results is empty) it falls back to num/err.
type fakeIssuer struct {
	title, body string
	labels      []string
	issueType   string
	calls       int
	results     []issueResult
	num         int
	err         error
}

func (f *fakeIssuer) CreateIssue(_ context.Context, title, body string, labels []string, issueType string) (int, error) {
	f.title, f.body, f.labels, f.issueType = title, body, labels, issueType
	f.calls++
	if f.calls <= len(f.results) {
		r := f.results[f.calls-1]
		return r.num, r.err
	}
	return f.num, f.err
}

// noDelay makes a worker's retries fire back-to-back so tests don't wait on real backoff.
func noDelay(w *Worker) *Worker {
	w.sleep = func(time.Duration) {}
	return w
}

func TestProcessStoresScreenshotsAndOpensIssue(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	fake := &fakeIssuer{num: 11}
	w := New(queue.New(1), fake, st, "https://reporting.example/")

	job := queue.Job{ID: "r1", Payload: report.Payload{
		Comment: "boom\nmore details",
		OS:      "linux",
		Arch:    "amd64",
		OpenDocuments: []report.DocumentInfo{
			{Name: "widget", Type: "part", Active: true, Content: "schemaVersion: 2\n"},
			{Name: "rig", Type: "assembly", Content: "schemaVersion: 2\n"},
		},
		WindowPNG:   []byte("WINDOW"),
		ViewportPNG: []byte("VIEWPORT"),
	}}
	if err := w.process(context.Background(), job); err != nil {
		t.Fatalf("process: %v", err)
	}

	// Issue recorded with the returned number.
	refs, _ := st.Reports()
	if len(refs) != 1 || refs[0].Issue.Number != 11 {
		t.Fatalf("issue meta = %+v, want one #11", refs)
	}
	// Title from the active document (not the comment); body embeds the screenshot URLs.
	if fake.title != "Bug report — widget" {
		t.Errorf("title = %q", fake.title)
	}
	if !strings.Contains(fake.body, "https://reporting.example/r/r1/window.png") {
		t.Errorf("body missing window screenshot URL:\n%s", fake.body)
	}
	// Issue type and labels are stamped.
	if fake.issueType != "Bug" {
		t.Errorf("issue type = %q, want Bug", fake.issueType)
	}
	wantLabels := map[string]bool{"user-submitted": true, "linux-amd64": true, "part-document": true}
	for _, l := range fake.labels {
		delete(wantLabels, l)
	}
	if len(wantLabels) != 0 {
		t.Errorf("labels = %v, missing %v", fake.labels, wantLabels)
	}
	// The active document's YAML is rendered as a code block.
	if !strings.Contains(fake.body, "Active document — widget") || !strings.Contains(fake.body, "```yaml") {
		t.Errorf("body missing active-document YAML block:\n%s", fake.body)
	}
}

func TestProcessRetriesTransientFailureThenSucceeds(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	fake := &fakeIssuer{results: []issueResult{
		{err: &github.StatusError{Code: 503}},
		{num: 22},
	}}
	w := noDelay(New(queue.New(1), fake, st, "https://reporting.example/"))

	job := queue.Job{ID: "r1", Payload: report.Payload{Comment: "boom"}}
	if err := w.process(context.Background(), job); err != nil {
		t.Fatalf("process: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("CreateIssue called %d times, want 2 (one retry)", fake.calls)
	}
	refs, _ := st.Reports()
	if len(refs) != 1 || refs[0].Issue.Number != 22 {
		t.Fatalf("issue meta = %+v, want one #22", refs)
	}
}

func TestProcessDoesNotRetryPermanentFailure(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	fake := &fakeIssuer{err: &github.StatusError{Code: 422, Body: "body is too long"}}
	w := noDelay(New(queue.New(1), fake, st, "https://reporting.example/"))

	job := queue.Job{ID: "r2", Payload: report.Payload{Comment: "boom"}}
	if err := w.process(context.Background(), job); err == nil {
		t.Fatal("process: want error, got nil")
	}
	if fake.calls != 1 {
		t.Errorf("CreateIssue called %d times, want 1 (a 4xx should fail fast)", fake.calls)
	}
	if _, err := st.ReadDeadLetter("r2"); err != nil {
		t.Errorf("ReadDeadLetter: %v, want the report dead-lettered", err)
	}
}

func TestProcessDeadLettersAfterExhaustingRetries(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	fake := &fakeIssuer{err: &github.StatusError{Code: 500, Body: "upstream error"}}
	w := noDelay(New(queue.New(1), fake, st, "https://reporting.example/"))

	job := queue.Job{ID: "r3", Payload: report.Payload{Comment: "boom", OS: "linux"}}
	err = w.process(context.Background(), job)
	if err == nil {
		t.Fatal("process: want error, got nil")
	}
	if fake.calls != maxCreateAttempts {
		t.Errorf("CreateIssue called %d times, want %d", fake.calls, maxCreateAttempts)
	}
	if !strings.Contains(err.Error(), "dead-letter") {
		t.Errorf("process error = %q, want it to mention the dead letter", err)
	}

	dl, err := st.ReadDeadLetter("r3")
	if err != nil {
		t.Fatalf("ReadDeadLetter: %v", err)
	}
	if dl.Payload.OS != "linux" {
		t.Errorf("dead letter payload = %+v, want the original report", dl.Payload)
	}
	if !strings.Contains(dl.Error, "upstream error") {
		t.Errorf("dead letter error = %q, want it to record the GitHub failure", dl.Error)
	}
}

func TestRetryableClassifiesByStatusCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"server error is retryable", &github.StatusError{Code: 503}, true},
		{"client error is not retryable", &github.StatusError{Code: 422}, false},
		{"non-HTTP error is retryable", errors.New("dial tcp: connection refused"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := retryable(c.err); got != c.want {
				t.Errorf("retryable(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestProcessReportsWhenDeadLetterSaveAlsoFails(t *testing.T) {
	dir := t.TempDir()
	st, err := storage.New(dir)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	fake := &fakeIssuer{err: &github.StatusError{Code: 422}}
	w := noDelay(New(queue.New(1), fake, st, "https://reporting.example/"))

	job := queue.Job{ID: "r4", Payload: report.Payload{Comment: "boom"}}
	// Make the report directory (already created by SaveScreenshots) unwritable, so the
	// dead-letter write itself fails too.
	reportDir := filepath.Join(dir, "r4")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(reportDir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer func() { _ = os.Chmod(reportDir, 0o755) }() // let t.TempDir() clean up

	err = w.process(context.Background(), job)
	if err == nil {
		t.Fatal("process: want error, got nil")
	}
	if !strings.Contains(err.Error(), "dead-letter save also failed") {
		t.Errorf("process error = %q, want it to mention the dead-letter save failure", err)
	}
}
