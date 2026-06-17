// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"oblikovati.org/reporting/internal/auth"
	"oblikovati.org/reporting/internal/queue"
)

func postReport(t *testing.T, h http.Handler, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/report", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestReportAcceptedAndQueuedWithValidToken(t *testing.T) {
	jobs := queue.New(4)
	h := New(jobs, http.NotFoundHandler(), 1<<20).Handler()

	body := []byte(`{"comment":"hello"}`)
	rec := postReport(t, h, body, auth.Token(body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	select {
	case job := <-jobs.Jobs():
		if job.Payload.Comment != "hello" || job.ID == "" {
			t.Errorf("queued job = %+v", job)
		}
	default:
		t.Error("no job enqueued")
	}
}

func TestReportRejectsBadToken(t *testing.T) {
	jobs := queue.New(4)
	h := New(jobs, http.NotFoundHandler(), 1<<20).Handler()

	rec := postReport(t, h, []byte(`{"comment":"x"}`), "deadbeef")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(jobs.Jobs()) != 0 {
		t.Error("rejected report should not enqueue")
	}
}

func TestReportRejectsOversizeBody(t *testing.T) {
	jobs := queue.New(4)
	h := New(jobs, http.NotFoundHandler(), 8).Handler() // 8-byte cap

	body := []byte(`{"comment":"this is well over eight bytes"}`)
	rec := postReport(t, h, body, auth.Token(body))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	h := New(queue.New(1), http.NotFoundHandler(), 1<<20).Handler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ok") {
		t.Errorf("healthz = %d %s", rec.Code, rec.Body)
	}
}
