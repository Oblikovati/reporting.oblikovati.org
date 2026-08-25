// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"fmt"
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

func TestIssueBodyStaysUnderGitHubLimit(t *testing.T) {
	const githubBodyLimit = 65536
	docYAML := strings.Repeat("x: 1\n", 20000) // ~100KB, alone bigger than the whole limit
	docs := make([]report.DocumentInfo, 20)
	for i := range docs {
		docs[i] = report.DocumentInfo{Name: fmt.Sprintf("doc%d", i), Type: "part", Active: i == 0, Content: docYAML}
	}
	events := make([]report.TransactionEvent, 500)
	for i := range events {
		events[i] = report.TransactionEvent{Time: "09:00:00", Document: "main", Label: "Extrude", Recipe: strings.Repeat("y: 2\n", 200)}
	}

	w := New(nil, nil, nil, "https://reporting.example")
	body := w.issueBody("bigrep", report.Payload{
		Comment:        strings.Repeat("why did this happen? ", 2000),
		OpenDocuments:  docs,
		TransactionLog: events,
		UserSettings:   strings.Repeat("z: 3\n", 5000),
	})

	if len(body) > githubBodyLimit {
		t.Fatalf("body is %d bytes, exceeds GitHub's %d-character issue-body limit", len(body), githubBodyLimit)
	}
	if !strings.Contains(body, "report id: `bigrep`") {
		t.Error("body missing the trailing report-id footer (it must survive truncation)")
	}
	if !strings.Contains(body, "exceeds GitHub's size limit") {
		t.Error("body missing an omission note explaining what was dropped")
	}
}

func TestIssueBodyOmitsOversizedDocumentWholeNotTruncated(t *testing.T) {
	huge := strings.Repeat("x: 1\n", 20000) // bigger than the whole budget on its own
	w := New(nil, nil, nil, "https://reporting.example")
	body := w.issueBody("r", report.Payload{
		OpenDocuments: []report.DocumentInfo{
			{Name: "small", Type: "part", Active: true, Content: "schemaVersion: 2\n"},
			{Name: "huge", Type: "part", Content: huge},
		},
	})
	if !strings.Contains(body, "schemaVersion: 2") {
		t.Error("small document should still be rendered in full")
	}
	if strings.Contains(body, "x: 1\n") {
		t.Error("oversized document must be omitted whole, not truncated mid-YAML")
	}
	if !strings.Contains(body, "huge") {
		t.Error("the omitted document's name should be named in the omission note")
	}
}

func TestDocLabelFallsBackToPlaceholderWhenUnnamed(t *testing.T) {
	if got := docLabel(report.DocumentInfo{Name: "  "}); got != "unnamed document" {
		t.Errorf("docLabel(unnamed) = %q, want placeholder", got)
	}
	if got := docLabel(report.DocumentInfo{Name: "widget"}); got != "widget" {
		t.Errorf("docLabel(widget) = %q", got)
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
