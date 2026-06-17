// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"fmt"
	"strings"

	"oblikovati.org/reporting/internal/report"
)

// titleMaxLen keeps the issue title to a single readable line.
const titleMaxLen = 80

// issueTitle derives a one-line issue title from the user's comment, falling back to a
// generic title when the comment is empty.
func issueTitle(p report.Payload) string {
	line := firstLine(p.Comment)
	if line == "" {
		return "Bug report (no description)"
	}
	return "Bug report: " + truncate(line, titleMaxLen)
}

// issueBody renders the report as GitHub-flavoured markdown: the user's comment, the
// embedded screenshots (served by this service), and collapsible diagnostics.
func (w *Worker) issueBody(id string, p report.Payload) string {
	var b strings.Builder
	writeComment(&b, p.Comment)
	w.writeScreenshots(&b, id, p)
	writeEnvironment(&b, p)
	writeOpenDocuments(&b, p)
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
	fmt.Fprintf(b, "| | |\n|---|---|\n")
	fmt.Fprintf(b, "| OS / Arch | %s / %s |\n", mdCell(p.OS), mdCell(p.Arch))
	fmt.Fprintf(b, "| Version | %s |\n", mdCell(p.AppVersion))
	fmt.Fprintf(b, "| Commit | %s |\n", mdCell(p.AppCommit))
	fmt.Fprintf(b, "| Build date | %s |\n", mdCell(p.AppBuildDate))
}

func writeOpenDocuments(b *strings.Builder, p report.Payload) {
	if len(p.OpenDocuments) == 0 {
		return
	}
	b.WriteString("\n### Open documents\n\n")
	for _, d := range p.OpenDocuments {
		dirty := ""
		if d.Dirty {
			dirty = " *(unsaved)*"
		}
		fmt.Fprintf(b, "- **%s** (%s) — `%s`%s\n", mdCell(d.Name), mdCell(d.Type), d.Path, dirty)
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

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
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
