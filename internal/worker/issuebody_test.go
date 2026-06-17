// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"strings"
	"testing"

	"oblikovati.org/reporting/internal/report"
)

func TestIssueTitleFromActiveDocumentNotComment(t *testing.T) {
	// No documents → a generic title (the comment is optional and never the title).
	if got := issueTitle(report.Payload{Comment: "this should not be the title"}); got != "User-submitted bug report" {
		t.Errorf("title = %q", got)
	}
	// With an active document, the title names it.
	p := report.Payload{
		Comment:       "ignored for the title",
		OpenDocuments: []report.DocumentInfo{{Name: "bracket", Type: "part", Active: true}},
	}
	if got := issueTitle(p); got != "Bug report — bracket" {
		t.Errorf("title = %q", got)
	}
}

func TestIssueLabels(t *testing.T) {
	p := report.Payload{
		OS:   "windows",
		Arch: "arm64",
		OpenDocuments: []report.DocumentInfo{
			{Name: "a", Type: "assembly", Active: true},
			{Name: "b", Type: "part"},
		},
	}
	got := strings.Join(issueLabels(p), ",")
	for _, want := range []string{"user-submitted", "windows-arm64", "assembly-document"} {
		if !strings.Contains(got, want) {
			t.Errorf("labels %q missing %q", got, want)
		}
	}
}

func TestIssueLabelsOmitUnknownDocType(t *testing.T) {
	p := report.Payload{OS: "linux", Arch: "amd64"} // no documents
	for _, l := range issueLabels(p) {
		if strings.HasSuffix(l, "-document") {
			t.Errorf("unexpected document label %q with no active document", l)
		}
	}
}

func TestIssueBodyRendersDocumentYAMLActiveFirst(t *testing.T) {
	w := New(nil, nil, nil, "https://reporting.example")
	body := w.issueBody("rep9", report.Payload{
		Comment:        "crash",
		OS:             "linux",
		Arch:           "amd64",
		AppVersion:     "0.16.0",
		TransactionLog: []report.TransactionEvent{{Time: "09:00:12", Document: "main", Label: "Extrude", Recipe: "features:\n  - extrude\n"}},
		UserSettings:   "general:\n  gridVisible: true\n",
		OpenDocuments: []report.DocumentInfo{
			{Name: "main|part", Type: "part", Path: "/tmp/main.opd", Dirty: true, Active: true, Content: "schemaVersion: 2\ndocumentType: 1\n"},
			{Name: "sub", Type: "part", Path: "/tmp/sub.opd", Content: "schemaVersion: 2\n"},
		},
	})
	for _, want := range []string{
		"### What happened", "crash", "linux", "amd64", "report id: `rep9`",
		"Active document — main\\|part", // pipe escaped, active labelled
		"Other document — sub",
		"```yaml\nschemaVersion: 2\ndocumentType: 1\n```",
		"Transaction log (1 events since launch)",
		"**Extrude**",            // the transaction label
		"features:\n  - extrude", // the replayable recipe payload
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	// Active document's block must appear before the other document's.
	if strings.Index(body, "Active document") > strings.Index(body, "Other document — sub") {
		t.Error("active document should be rendered first")
	}
}

func TestIssueBodyNoteWhenContentUnavailable(t *testing.T) {
	w := New(nil, nil, nil, "https://reporting.example")
	body := w.issueBody("r", report.Payload{
		OpenDocuments: []report.DocumentInfo{{Name: "x", Type: "part", Active: true}},
	})
	if !strings.Contains(body, "document content unavailable") {
		t.Errorf("expected unavailable note:\n%s", body)
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
