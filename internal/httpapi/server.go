// SPDX-License-Identifier: Apache-2.0

// Package httpapi is the ingest surface: it accepts a CRC-authorized report, enqueues it for
// the worker, serves the stored screenshots, and answers a health probe. It does no GitHub
// or disk-heavy work inline — POST returns 202 as soon as the report is queued.
package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"oblikovati.org/reporting/internal/auth"
	"oblikovati.org/reporting/internal/queue"
	"oblikovati.org/reporting/internal/report"
)

// Server wires the ingest routes over the queue and screenshot store.
type Server struct {
	jobs    *queue.Queue
	files   http.Handler
	maxBody int64
}

// New builds the ingest server. files is the screenshot file handler (storage.FileServer).
func New(jobs *queue.Queue, files http.Handler, maxBody int64) *Server {
	return &Server{jobs: jobs, files: files, maxBody: maxBody}
}

// Handler returns the router: POST /report, GET /healthz, and GET /r/ for screenshots.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /report", s.handleReport)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /r/", http.StripPrefix("/r/", s.files))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"status":"ok"}`)
}

// handleReport validates the CRC token over the exact bytes received, then queues the report.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return
		}
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	if !auth.Verify(body, r.Header.Get("Authorization")) {
		writeError(w, http.StatusUnauthorized, "invalid authorization token")
		return
	}
	var p report.Payload
	if err := json.Unmarshal(body, &p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not allocate report id")
		return
	}
	if !s.jobs.Enqueue(queue.Job{ID: id, Payload: p}) {
		writeError(w, http.StatusServiceUnavailable, "server busy, please retry")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "queued"})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// newID returns a random 128-bit hex identifier used as the report's directory name and the
// public screenshot path segment.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
