// SPDX-License-Identifier: Apache-2.0

// Package queue is the in-memory hand-off between the HTTP ingest and the background worker:
// a buffered channel of jobs. It is deliberately not durable — a thin shim, by design — so a
// restart drops anything still queued; the screenshot-cleanup state that must survive lives
// on the storage volume instead.
package queue

import "oblikovati.org/reporting/internal/report"

// Job is one accepted report awaiting issue creation.
type Job struct {
	ID      string
	Payload report.Payload
}

// Queue is a bounded FIFO of jobs.
type Queue struct {
	ch chan Job
}

// New returns a queue holding up to size jobs before Enqueue starts reporting full.
func New(size int) *Queue {
	if size < 1 {
		size = 1
	}
	return &Queue{ch: make(chan Job, size)}
}

// Enqueue adds a job without blocking, returning false when the queue is full so the caller
// can shed load (HTTP 503) rather than stall the request.
func (q *Queue) Enqueue(j Job) bool {
	select {
	case q.ch <- j:
		return true
	default:
		return false
	}
}

// Jobs is the receive side the worker ranges over.
func (q *Queue) Jobs() <-chan Job { return q.ch }
