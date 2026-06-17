// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"fmt"
	"strings"

	"oblikovati.org/reporting/internal/report"
)

// titleMaxLen keeps the issue title to a single readable line.
const titleMaxLen = 80

// issueTitle derives a stable one-line title. The user's comment is optional, so the title
// is NOT taken from it; it names the active document instead, falling back to a generic
// title when no document is open.
func issueTitle(p report.Payload) string {
	if d, ok := activeDoc(p); ok && strings.TrimSpace(d.Name) != "" {
		return "Bug report — " + truncate(d.Name, titleMaxLen)
	}
	return "User-submitted bug report"
}

// issueLabels builds the labels for a report: a constant "user-submitted", an
// "<os>-<arch>" platform label, and the active document's kind as "<type>-document"
// (e.g. "part-document"). GitHub creates any that do not yet exist.
func issueLabels(p report.Payload) []string {
	labels := []string{"user-submitted"}
	if p.OS != "" && p.Arch != "" {
		labels = append(labels, p.OS+"-"+p.Arch)
	}
	if t := activeDocType(p); t != "" {
		labels = append(labels, t+"-document")
	}
	return labels
}

// activeDocType is the active document's kind (e.g. "part"), or "" when there is no usable
// active document.
func activeDocType(p report.Payload) string {
	if d, ok := activeDoc(p); ok && d.Type != "" && d.Type != "unknown" {
		return d.Type
	}
	return ""
}

// activeDoc returns the active open document (the host sends it first and flags it).
func activeDoc(p report.Payload) (report.DocumentInfo, bool) {
	for _, d := range p.OpenDocuments {
		if d.Active {
			return d, true
		}
	}
	return report.DocumentInfo{}, false
}

// issueBody renders the report as GitHub-flavoured markdown: the user's comment, the
// embedded screenshots, the open documents as YAML code blocks (active first), and the
// remaining diagnostics.
func (w *Worker) issueBody(id string, p report.Payload) string {
	var b strings.Builder
	writeComment(&b, p.Comment)
	w.writeScreenshots(&b, id, p)
	writeEnvironment(&b, p)
	writeDocuments(&b, p)
	writeTransactionLog(&b, p)
	writeSettings(&b, p)
	fmt.Fprintf(&b, "\n<sub>report id: `%s`</sub>\n", id)
	return b.String()
}

func writeComment(b *strings.Builder, comment string) {
	b.WriteString("### What happened\n\n")
	if strings.TrimSpace(comment) == "" {
		b.WriteString("_No description provided._\n")
		return
	}
	b.WriteString(comment)
	b.WriteString("\n")
}

// writeScreenshots embeds the window and viewport images by URL. GitHub fetches and caches
// them, so they render in the issue as long as this service is serving them.
func (w *Worker) writeScreenshots(b *strings.Builder, id string, p report.Payload) {
	if len(p.WindowPNG) == 0 && len(p.ViewportPNG) == 0 {
		return
	}
	b.WriteString("\n### Screenshots\n\n")
	base := strings.TrimRight(w.baseURL, "/")
	if len(p.WindowPNG) > 0 {
		fmt.Fprintf(b, "**Application window**\n\n![window](%s/r/%s/window.png)\n\n", base, id)
	}
	if len(p.ViewportPNG) > 0 {
		fmt.Fprintf(b, "**Viewport**\n\n![viewport](%s/r/%s/viewport.png)\n\n", base, id)
	}
}

func writeEnvironment(b *strings.Builder, p report.Payload) {
	b.WriteString("\n### Environment\n\n")
	b.WriteString("| | |\n|---|---|\n")
	fmt.Fprintf(b, "| OS / Arch | %s / %s |\n", mdCell(p.OS), mdCell(p.Arch))
	fmt.Fprintf(b, "| Version | %s |\n", mdCell(p.AppVersion))
	fmt.Fprintf(b, "| Commit | %s |\n", mdCell(p.AppCommit))
	fmt.Fprintf(b, "| Build date | %s |\n", mdCell(p.AppBuildDate))
}

// writeDocuments renders each open document's .obk YAML as a code block, the active document
// first, so a triager can reproduce from the exact files. A document whose content could
// not be captured is noted rather than skipped.
func writeDocuments(b *strings.Builder, p report.Payload) {
	if len(p.OpenDocuments) == 0 {
		return
	}
	b.WriteString("\n### Open documents\n")
	for _, d := range p.OpenDocuments {
		heading := "Other document"
		if d.Active {
			heading = "Active document"
		}
		dirty := ""
		if d.Dirty {
			dirty = " *(unsaved)*"
		}
		fmt.Fprintf(b, "\n**%s — %s** (%s) `%s`%s\n\n", heading, mdCell(d.Name), mdCell(d.Type), d.Path, dirty)
		if strings.TrimSpace(d.Content) == "" {
			b.WriteString("_document content unavailable_\n")
			continue
		}
		b.WriteString("```yaml\n")
		b.WriteString(d.Content)
		if !strings.HasSuffix(d.Content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}
}

func writeTransactionLog(b *strings.Builder, p report.Payload) {
	if len(p.TransactionLog) == 0 {
		return
	}
	b.WriteString("\n<details><summary>Transaction log</summary>\n\n")
	for i, step := range p.TransactionLog {
		fmt.Fprintf(b, "%d. %s\n", i+1, step)
	}
	b.WriteString("\n</details>\n")
}

func writeSettings(b *strings.Builder, p report.Payload) {
	if strings.TrimSpace(p.UserSettings) == "" {
		return
	}
	b.WriteString("\n<details><summary>User settings</summary>\n\n```yaml\n")
	b.WriteString(p.UserSettings)
	if !strings.HasSuffix(p.UserSettings, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n\n</details>\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// mdCell makes a value safe for a one-line markdown table cell, collapsing newlines and
// escaping the pipe that would otherwise start a new column. Empty becomes an en dash.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	if strings.TrimSpace(s) == "" {
		return "–"
	}
	return s
}
