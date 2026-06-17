// SPDX-License-Identifier: Apache-2.0

// Package report defines the bug-report payload the Oblikovati application POSTs to this
// service. Its JSON shape is the contract with the app; by project decision it is
// duplicated here rather than shared through a module, so the json tags MUST match the
// app's report.Payload exactly (a round-trip test on each side guards against drift).
package report

// Payload is one submitted bug report. The two screenshots arrive base64-encoded because
// they are []byte in JSON.
type Payload struct {
	Comment        string         `json:"comment"`
	OS             string         `json:"os"`
	Arch           string         `json:"arch"`
	AppVersion     string         `json:"appVersion"`
	AppCommit      string         `json:"appCommit"`
	AppBuildDate   string         `json:"appBuildDate"`
	UserSettings   string         `json:"userSettings"`
	OpenDocuments  []DocumentInfo `json:"openDocuments"`
	TransactionLog []string       `json:"transactionLog"`
	WindowPNG      []byte         `json:"windowPng,omitempty"`
	ViewportPNG    []byte         `json:"viewportPng,omitempty"`
}

// DocumentInfo is one open document at the time of the report. Content is the document's
// full .obk YAML (the file as saved); the active document is sent first and has Active=true.
type DocumentInfo struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Dirty   bool   `json:"dirty"`
	Active  bool   `json:"active"`
	Content string `json:"content,omitempty"`
}
