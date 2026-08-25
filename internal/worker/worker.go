// SPDX-License-Identifier: Apache-2.0

// Package worker drains the in-memory queue: for each report it stores the screenshots,
// opens a GitHub issue whose body embeds the screenshot URLs and diagnostics, then records
// the issue number so the reconciler can later clean the images up.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"oblikovati.org/reporting/internal/github"
	"oblikovati.org/reporting/internal/queue"
	"oblikovati.org/reporting/internal/report"
	"oblikovati.org/reporting/internal/storage"
)

// issueAttemptTimeout bounds a single GitHub issue-creation HTTP call (one of up to
// maxCreateAttempts).
const issueAttemptTimeout = 20 * time.Second

// issueType is the GitHub repository issue type stamped on every report.
const issueType = "Bug"

// maxCreateAttempts bounds how many times CreateIssue is retried for a transient failure
// before the report is dead-lettered.
const maxCreateAttempts = 3

// retryBaseDelay is the backoff before the second attempt; it doubles each attempt after.
const retryBaseDelay = 2 * time.Second

// IssueCreator opens an issue and returns its number (the github.Client implements it).
type IssueCreator interface {
	CreateIssue(ctx context.Context, title, body string, labels []string, issueType string) (int, error)
}

// Worker processes jobs from a queue into GitHub issues.
type Worker struct {
	jobs    *queue.Queue
	gh      IssueCreator
	store   *storage.Store
	baseURL string
	sleep   func(time.Duration) // the retry backoff; real time.Sleep, faked in tests
}

// New builds a worker. baseURL is the public origin used to construct screenshot links.
func New(jobs *queue.Queue, gh IssueCreator, store *storage.Store, baseURL string) *Worker {
	return &Worker{jobs: jobs, gh: gh, store: store, baseURL: baseURL, sleep: time.Sleep}
}

// Run drains the queue until ctx is cancelled. One job at a time keeps GitHub's rate limit
// comfortable; the queue absorbs bursts.
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-w.jobs.Jobs():
			if err := w.process(ctx, job); err != nil {
				log.Printf("worker: report %s failed: %v", job.ID, err)
			}
		}
	}
}

// process stores the screenshots, opens the issue (retrying transient failures), and
// records its number. A report that still fails after retries is dead-lettered rather than
// dropped, so it is recoverable instead of silently lost.
func (w *Worker) process(ctx context.Context, job queue.Job) error {
	p := job.Payload
	if err := w.store.SaveScreenshots(job.ID, p.WindowPNG, p.ViewportPNG); err != nil {
		return err
	}
	number, err := w.createIssueWithRetry(ctx, job.ID, p)
	if err != nil {
		return w.deadLetter(job.ID, p, err)
	}
	return w.store.SaveIssueMeta(job.ID, storage.IssueMeta{Number: number, CreatedAt: time.Now().UTC()})
}

// createIssueWithRetry opens the GitHub issue, retrying a transient failure (network error
// or GitHub 5xx) with backoff up to maxCreateAttempts. A permanent failure (4xx: bad
// request, auth, validation) fails fast — retrying would just resend the same request.
func (w *Worker) createIssueWithRetry(ctx context.Context, id string, p report.Payload) (int, error) {
	title, body, labels := issueTitle(p), w.issueBody(id, p), issueLabels(p)
	var lastErr error
	for attempt := 1; attempt <= maxCreateAttempts; attempt++ {
		issueCtx, cancel := context.WithTimeout(ctx, issueAttemptTimeout)
		number, err := w.gh.CreateIssue(issueCtx, title, body, labels, issueType)
		cancel()
		if err == nil {
			return number, nil
		}
		lastErr = err
		if !retryable(err) || attempt == maxCreateAttempts {
			break
		}
		log.Printf("worker: report %s: create issue attempt %d/%d failed, retrying: %v", id, attempt, maxCreateAttempts, err)
		w.sleep(retryBaseDelay << (attempt - 1))
	}
	return 0, lastErr
}

// retryable reports whether err is worth another attempt.
func retryable(err error) bool {
	var se *github.StatusError
	if errors.As(err, &se) {
		return se.Code >= 500
	}
	return true // network-level failure or timeout: might clear up
}

// deadLetter is the last resort when issue creation exhausts its retries: it persists the
// full report so it can be recovered manually, and returns an error that says so — the
// caller's log line is the operator's signal that a report needs attention, not that it
// vanished.
func (w *Worker) deadLetter(id string, p report.Payload, cause error) error {
	if err := w.store.SaveDeadLetter(id, p, cause); err != nil {
		return fmt.Errorf("create issue failed (%w) and dead-letter save also failed: %v", cause, err)
	}
	return fmt.Errorf("create issue failed after retries, dead-lettered for manual recovery: %w", cause)
}
