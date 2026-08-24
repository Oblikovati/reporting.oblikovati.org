// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"fmt"
	"strings"

	"oblikovati.org/reporting/internal/report"
)

// titleMaxLen keeps the issue title to a single readable line.
const titleMaxLen = 80

// bodyMaxLen keeps the rendered issue body safely under GitHub's create-issue limit
// ("body is too long (maximum is 65536 characters)"). The margin covers the trailing
// footer, the omission notes below, and byte-vs-character counting slack for UTF-8 text.
const bodyMaxLen = 60000

// commentMaxLen bounds the free-text comment alone, so one huge paste can't consume the
// whole body budget before any diagnostics are written.
const commentMaxLen = 8000

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
// remaining diagnostics. The comment, screenshots, environment table and footer are small
// and fixed, so they are always written in full; documents, the transaction log and the
// settings are unbounded, so each gets whatever is left of bodyMaxLen after the sections
// before it, and anything that would not fit is omitted (never truncated mid-block) and
// named in a trailing note — see writeDocuments.
func (w *Worker) issueBody(id string, p report.Payload) string {
	var b strings.Builder
	writeComment(&b, truncate(p.Comment, commentMaxLen))
	w.writeScreenshots(&b, id, p)
	writeEnvironment(&b, p)
	footer := fmt.Sprintf("\n<sub>report id: `%s`</sub>\n", id)
	remaining := bodyMaxLen - b.Len() - len(footer)
	remaining = writeDocuments(&b, p, remaining)
	remaining = writeTransactionLog(&b, p, remaining)
	writeSettings(&b, p, remaining)
	b.WriteString(footer)
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
// first, so a triager can reproduce from the exact files, until budget runs out. A document
// is never truncated mid-YAML — one that would not fit whole is left out entirely and named
// in a trailing note instead, so every YAML block a triager sees is complete and trustworthy.
// A document whose content could not be captured is noted rather than skipped. Returns the
// budget left over for the sections after it.
func writeDocuments(b *strings.Builder, p report.Payload, budget int) int {
	if len(p.OpenDocuments) == 0 {
		return budget
	}
	b.WriteString("\n### Open documents\n")
	var omitted []string
	for _, d := range p.OpenDocuments {
		block := renderDocument(d)
		if len(block) > budget {
			omitted = append(omitted, docLabel(d))
			continue
		}
		b.WriteString(block)
		budget -= len(block)
	}
	writeOmittedNote(b, "open document(s)", omitted)
	return budget
}

func renderDocument(d report.DocumentInfo) string {
	var b strings.Builder
	heading := "Other document"
	if d.Active {
		heading = "Active document"
	}
	dirty := ""
	if d.Dirty {
		dirty = " *(unsaved)*"
	}
	fmt.Fprintf(&b, "\n**%s — %s** (%s) `%s`%s\n\n", heading, mdCell(d.Name), mdCell(d.Type), d.Path, dirty)
	if strings.TrimSpace(d.Content) == "" {
		b.WriteString("_document content unavailable_\n")
		return b.String()
	}
	b.WriteString("```yaml\n")
	b.WriteString(d.Content)
	if !strings.HasSuffix(d.Content, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n")
	return b.String()
}

// docLabel names a document in an omission note, falling back to a placeholder for the
// (unlikely) case the host sent one with no name.
func docLabel(d report.DocumentInfo) string {
	if strings.TrimSpace(d.Name) == "" {
		return "unnamed document"
	}
	return d.Name
}

// writeOmittedNote records what a size-budgeted section left out, if anything.
func writeOmittedNote(b *strings.Builder, what string, omitted []string) {
	if len(omitted) == 0 {
		return
	}
	fmt.Fprintf(b, "\n_%d %s omitted — report body exceeds GitHub's size limit: %s._\n",
		len(omitted), what, strings.Join(omitted, ", "))
}

// writeTransactionLog renders the transaction-manager events since the app opened, oldest
// first, each with its full recipe payload so the interaction sequence can be replayed
// precisely, until budget runs out. Like writeDocuments, an event is never truncated
// mid-recipe — one that would not fit whole is left out and counted in a trailing note, so
// every rendered recipe stays replayable. The whole log is collapsed, and each event's
// recipe is a nested collapsed YAML block, so a long session stays readable. Returns the
// budget left over for the sections after it.
func writeTransactionLog(b *strings.Builder, p report.Payload, budget int) int {
	if len(p.TransactionLog) == 0 {
		return budget
	}
	header := fmt.Sprintf("\n<details><summary>Transaction log (%d events since launch)</summary>\n\n", len(p.TransactionLog))
	const trailer = "\n</details>\n"
	if len(header)+len(trailer) > budget {
		fmt.Fprintf(b, "\n_transaction log omitted (%d events) — report body exceeds GitHub's size limit._\n", len(p.TransactionLog))
		return budget
	}
	b.WriteString(header)
	budget -= len(header) + len(trailer)
	omitted := 0
	for i, e := range p.TransactionLog {
		block := renderTransactionEvent(i, e)
		if len(block) > budget {
			omitted++
			continue
		}
		b.WriteString(block)
		budget -= len(block)
	}
	if omitted > 0 {
		fmt.Fprintf(b, "\n_%d more event(s) omitted — report body exceeds GitHub's size limit._\n", omitted)
	}
	b.WriteString(trailer)
	return budget
}

func renderTransactionEvent(i int, e report.TransactionEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d. `%s` %s — **%s**\n", i+1, e.Time, mdCell(e.Document), mdCell(e.Label))
	if strings.TrimSpace(e.Recipe) != "" {
		b.WriteString("<details><summary>recipe</summary>\n\n```yaml\n")
		b.WriteString(e.Recipe)
		if !strings.HasSuffix(e.Recipe, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n</details>\n")
	}
	return b.String()
}

// writeSettings renders the full user-settings YAML if it fits in what remains of the
// budget, or notes that it was omitted — never truncated mid-YAML, for the same reason as
// writeDocuments and writeTransactionLog. Settings is the lowest-priority section (last in
// issueBody), so it is the first to be dropped when a report is large.
func writeSettings(b *strings.Builder, p report.Payload, budget int) {
	if strings.TrimSpace(p.UserSettings) == "" {
		return
	}
	block := renderSettings(p.UserSettings)
	if len(block) > budget {
		b.WriteString("\n_user settings omitted — report body exceeds GitHub's size limit._\n")
		return
	}
	b.WriteString(block)
}

func renderSettings(settings string) string {
	var b strings.Builder
	b.WriteString("\n<details><summary>User settings</summary>\n\n```yaml\n")
	b.WriteString(settings)
	if !strings.HasSuffix(settings, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n\n</details>\n")
	return b.String()
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
