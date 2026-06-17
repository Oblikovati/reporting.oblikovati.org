// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"strings"
	"testing"

	"oblikovati.org/reporting/internal/queue"
	"oblikovati.org/reporting/internal/report"
	"oblikovati.org/reporting/internal/storage"
)

// fakeIssuer records the issue it was asked to create instead of calling GitHub.
type fakeIssuer struct {
	title, body string
	labels      []string
	issueType   string
	num         int
	err         error
}

func (f *fakeIssuer) CreateIssue(_ context.Context, title, body string, labels []string, issueType string) (int, error) {
	f.title, f.body, f.labels, f.issueType = title, body, labels, issueType
	return f.num, f.err
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
