// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSaveServeAndEnumerate(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := st.SaveScreenshots("rep1", []byte("WINDOW"), []byte("VIEWPORT")); err != nil {
		t.Fatalf("SaveScreenshots: %v", err)
	}
	if err := st.SaveIssueMeta("rep1", IssueMeta{Number: 42, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveIssueMeta: %v", err)
	}

	// The screenshots are served under /<id>/<file> by the file handler.
	srv := httptest.NewServer(http.StripPrefix("/r/", st.FileServer()))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/r/rep1/window.png")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(got) != "WINDOW" {
		t.Errorf("served window = %q (status %d)", got, resp.StatusCode)
	}

	refs, err := st.Reports()
	if err != nil {
		t.Fatalf("Reports: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != "rep1" || refs[0].Issue.Number != 42 {
		t.Fatalf("Reports = %+v, want one rep1/#42", refs)
	}
}

func TestReportsSkipsDirsWithoutIssueMeta(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Screenshots saved but no issue.json yet (mid-creation): not ready to reconcile.
	if err := st.SaveScreenshots("pending", []byte("X"), nil); err != nil {
		t.Fatalf("SaveScreenshots: %v", err)
	}
	refs, err := st.Reports()
	if err != nil {
		t.Fatalf("Reports: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("Reports = %+v, want none (no issue meta)", refs)
	}
}

func TestDeleteRemovesReport(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = st.SaveScreenshots("gone", []byte("X"), nil)
	_ = st.SaveIssueMeta("gone", IssueMeta{Number: 1})
	if err := st.Delete("gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	refs, _ := st.Reports()
	if len(refs) != 0 {
		t.Errorf("report still present after delete: %+v", refs)
	}
}
