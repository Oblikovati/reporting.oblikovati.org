// SPDX-License-Identifier: Apache-2.0

package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateIssueSendsTitleLabelsAndType(t *testing.T) {
	var gotAuth, gotPath string
	var in struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Labels []string `json:"labels"`
		Type   string   `json:"type"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &in)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"number":7}`)
	}))
	defer srv.Close()

	c := New("tok123", "Oblikovati", "Oblikovati", srv.Client())
	c.SetAPIBase(srv.URL)
	num, err := c.CreateIssue(context.Background(), "Bug report — widget", "body text",
		[]string{"user-submitted", "linux-amd64", "part-document"}, "Bug")
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if num != 7 {
		t.Errorf("number = %d, want 7", num)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotPath != "/repos/Oblikovati/Oblikovati/issues" {
		t.Errorf("path = %q", gotPath)
	}
	if in.Title != "Bug report — widget" {
		t.Errorf("title = %q", in.Title)
	}
	if in.Type != "Bug" {
		t.Errorf("type = %q, want Bug", in.Type)
	}
	if strings.Join(in.Labels, ",") != "user-submitted,linux-amd64,part-document" {
		t.Errorf("labels = %v", in.Labels)
	}
}

func TestIssueStateReadsState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/issues/9") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"state":"closed"}`)
	}))
	defer srv.Close()

	c := New("tok", "o", "r", srv.Client())
	c.SetAPIBase(srv.URL)
	state, err := c.IssueState(context.Background(), 9)
	if err != nil {
		t.Fatalf("IssueState: %v", err)
	}
	if state != "closed" {
		t.Errorf("state = %q, want closed", state)
	}
}

func TestCreateIssueErrorsOnNon201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
	}))
	defer srv.Close()

	c := New("tok", "o", "r", srv.Client())
	c.SetAPIBase(srv.URL)
	if _, err := c.CreateIssue(context.Background(), "t", "b", nil, ""); err == nil {
		t.Fatal("want error on 401")
	}
}

func TestCreateIssueErrorExposesStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"message":"upstream timeout"}`)
	}))
	defer srv.Close()

	c := New("tok", "o", "r", srv.Client())
	c.SetAPIBase(srv.URL)
	_, err := c.CreateIssue(context.Background(), "t", "b", nil, "")
	if err == nil {
		t.Fatal("want error on 503")
	}
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *StatusError", err)
	}
	if se.Code != http.StatusServiceUnavailable {
		t.Errorf("StatusError.Code = %d, want %d", se.Code, http.StatusServiceUnavailable)
	}
}
