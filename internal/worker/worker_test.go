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
	num         int
	err         error
}

func (f *fakeIssuer) CreateIssue(_ context.Context, title, body string) (int, error) {
	f.title, f.body = title, body
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
		Comment:     "boom\nmore details",
		OS:          "linux",
		Arch:        "amd64",
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
	// Title from the first comment line; body embeds the served screenshot URLs.
	if !strings.HasPrefix(fake.title, "Bug report: boom") {
		t.Errorf("title = %q", fake.title)
	}
	if !strings.Contains(fake.body, "https://reporting.example/r/r1/window.png") {
		t.Errorf("body missing window screenshot URL:\n%s", fake.body)
	}
	if !strings.Contains(fake.body, "https://reporting.example/r/r1/viewport.png") {
		t.Errorf("body missing viewport screenshot URL:\n%s", fake.body)
	}
}
