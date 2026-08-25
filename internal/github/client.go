// SPDX-License-Identifier: Apache-2.0

// Package github is a thin REST client for the two operations this service needs: opening a
// bug issue and reading an issue's open/closed state. The transport is injected behind a
// one-method seam so the worker and reconciler are testable against an httptest server.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// defaultAPIBase is the public GitHub REST API host.
const defaultAPIBase = "https://api.github.com"

// maxBody caps the response we read from GitHub.
const maxBody = 1 << 20

// httpDoer is the single method of *http.Client the client needs (so tests can fake it).
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// StatusError reports a non-2xx HTTP response, exposing the status code so a caller (the
// worker's retry logic) can tell a transient failure (5xx, worth retrying) from a
// permanent one (4xx: bad request, auth, validation — retrying sends the same request).
type StatusError struct {
	Method, Endpoint string
	Code             int
	Body             string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("github: %s %q: %d: %s", e.Method, e.Endpoint, e.Code, e.Body)
}

// Client talks to one repository's issues with a personal access token.
type Client struct {
	token, owner, repo string
	apiBase            string
	http               httpDoer
}

// New returns a client for owner/repo authenticating with token over doer.
func New(token, owner, repo string, doer httpDoer) *Client {
	return &Client{token: token, owner: owner, repo: repo, apiBase: defaultAPIBase, http: doer}
}

// SetAPIBase overrides the API host (tests point it at an httptest server).
func (c *Client) SetAPIBase(base string) { c.apiBase = base }

// CreateIssue opens an issue and returns its number. labels are attached (GitHub creates
// any that do not yet exist, given push access); issueType sets the repository issue type
// (e.g. "Bug") and is omitted when empty.
func (c *Client) CreateIssue(ctx context.Context, title, body string, labels []string, issueType string) (int, error) {
	payload := map[string]any{"title": title, "body": body}
	if len(labels) > 0 {
		payload["labels"] = labels
	}
	if issueType != "" {
		payload["type"] = issueType
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("github: marshal issue: %w", err)
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues", c.apiBase, c.owner, c.repo)
	raw, err := c.do(ctx, http.MethodPost, endpoint, reqBody, http.StatusCreated)
	if err != nil {
		return 0, err
	}
	var out struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, fmt.Errorf("github: parse created issue: %w", err)
	}
	return out.Number, nil
}

// IssueState returns "open" or "closed" for an issue number.
func (c *Client) IssueState(ctx context.Context, number int) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.apiBase, c.owner, c.repo, number)
	raw, err := c.do(ctx, http.MethodGet, endpoint, nil, http.StatusOK)
	if err != nil {
		return "", err
	}
	var out struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("github: parse issue %d: %w", number, err)
	}
	return out.State, nil
}

// do performs an authenticated request and returns the capped body, erroring when the
// status is not the expected one.
func (c *Client) do(ctx context.Context, method, endpoint string, body []byte, want int) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, rdr)
	if err != nil {
		return nil, fmt.Errorf("github: build request for %q: %w", endpoint, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: %s %q: %w", method, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode != want {
		return nil, &StatusError{Method: method, Endpoint: endpoint, Code: resp.StatusCode, Body: string(bytes.TrimSpace(raw))}
	}
	return raw, nil
}
