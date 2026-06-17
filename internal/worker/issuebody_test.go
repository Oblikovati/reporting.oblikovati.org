// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"strings"
	"testing"

	"oblikovati.org/reporting/internal/report"
)

func TestIssueTitleFallsBackWhenNoComment(t *testing.T) {
	if got := issueTitle(report.Payload{}); got != "Bug report (no description)" {
		t.Errorf("title = %q", got)
	}
	if got := issueTitle(report.Payload{Comment: "first line\nsecond"}); got != "Bug report: first line" {
		t.Errorf("title = %q", got)
	}
}

func TestIssueBodyRendersDiagnostics(t *testing.T) {
	w := New(nil, nil, nil, "https://reporting.example")
	body := w.issueBody("rep9", report.Payload{
		Comment:        "crash",
		OS:             "linux",
		Arch:           "amd64",
		AppVersion:     "0.16.0",
		OpenDocuments:  []report.DocumentInfo{{Name: "part|1", Type: "Part", Path: "/tmp/p.obk", Dirty: true}},
		TransactionLog: []string{"Sketch", "Extrude"},
		UserSettings:   "general:\n  grid: true\n",
	})
	for _, want := range []string{"### What happened", "linux", "amd64", "Extrude", "```yaml", "report id: `rep9`"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	// A pipe in a table cell must be escaped so it doesn't start a new column.
	if !strings.Contains(body, `part\|1`) {
		t.Errorf("pipe not escaped in document name:\n%s", body)
	}
}

func TestMdCellEscapesAndDefaults(t *testing.T) {
	if got := mdCell(""); got != "–" {
		t.Errorf("empty cell = %q, want en dash", got)
	}
	if got := mdCell("a|b\nc"); got != `a\|b c` {
		t.Errorf("escaped cell = %q", got)
	}
}
