// SPDX-License-Identifier: Apache-2.0

// Package worker drains the in-memory queue: for each report it stores the screenshots,
// opens a GitHub issue whose body embeds the screenshot URLs and diagnostics, then records
// the issue number so the reconciler can later clean the images up.
package worker

import (
	"context"
	"log"
	"time"

	"oblikovati.org/reporting/internal/queue"
	"oblikovati.org/reporting/internal/storage"
)

// issueTimeout bounds a single GitHub issue creation.
const issueTimeout = 20 * time.Second

// issueType is the GitHub repository issue type stamped on every report.
const issueType = "Bug"

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
}

// New builds a worker. baseURL is the public origin used to construct screenshot links.
func New(jobs *queue.Queue, gh IssueCreator, store *storage.Store, baseURL string) *Worker {
	return &Worker{jobs: jobs, gh: gh, store: store, baseURL: baseURL}
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

// process stores the screenshots, opens the issue, and records its number.
func (w *Worker) process(ctx context.Context, job queue.Job) error {
	p := job.Payload
	if err := w.store.SaveScreenshots(job.ID, p.WindowPNG, p.ViewportPNG); err != nil {
		return err
	}
	issueCtx, cancel := context.WithTimeout(ctx, issueTimeout)
	defer cancel()
	number, err := w.gh.CreateIssue(issueCtx, issueTitle(p), w.issueBody(job.ID, p), issueLabels(p), issueType)
	if err != nil {
		return err
	}
	return w.store.SaveIssueMeta(job.ID, storage.IssueMeta{Number: number, CreatedAt: time.Now().UTC()})
}
