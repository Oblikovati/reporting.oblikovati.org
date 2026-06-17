// SPDX-License-Identifier: Apache-2.0

// Package reconciler periodically reconciles stored screenshots against their GitHub issue:
// once an issue is closed (the bug is triaged/fixed), its screenshots are no longer needed,
// so the report directory is deleted. State lives on the storage volume, so this works
// correctly across restarts.
package reconciler

import (
	"context"
	"log"
	"time"

	"oblikovati.org/reporting/internal/storage"
)

// sweepTimeout bounds a single issue-state query.
const sweepTimeout = 15 * time.Second

// IssueStater reads an issue's open/closed state (the github.Client implements it).
type IssueStater interface {
	IssueState(ctx context.Context, number int) (string, error)
}

// Reconciler deletes stored screenshots whose issue has closed.
type Reconciler struct {
	store    *storage.Store
	gh       IssueStater
	interval time.Duration
}

// New builds a reconciler that sweeps every interval.
func New(store *storage.Store, gh IssueStater, interval time.Duration) *Reconciler {
	return &Reconciler{store: store, gh: gh, interval: interval}
}

// Run sweeps once on start and then every interval until ctx is cancelled.
func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	r.Sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Sweep(ctx)
		}
	}
}

// Sweep checks every stored report once, deleting those whose issue is closed. It is
// exported so a test can drive a single pass deterministically.
func (r *Reconciler) Sweep(ctx context.Context) {
	refs, err := r.store.Reports()
	if err != nil {
		log.Printf("reconciler: list reports: %v", err)
		return
	}
	for _, ref := range refs {
		r.reconcile(ctx, ref)
	}
}

func (r *Reconciler) reconcile(ctx context.Context, ref storage.Ref) {
	stateCtx, cancel := context.WithTimeout(ctx, sweepTimeout)
	defer cancel()
	state, err := r.gh.IssueState(stateCtx, ref.Issue.Number)
	if err != nil {
		log.Printf("reconciler: issue %d state: %v", ref.Issue.Number, err)
		return
	}
	if state != "closed" {
		return
	}
	if err := r.store.Delete(ref.ID); err != nil {
		log.Printf("reconciler: delete report %s: %v", ref.ID, err)
		return
	}
	log.Printf("reconciler: deleted screenshots for report %s (issue #%d closed)", ref.ID, ref.Issue.Number)
}
