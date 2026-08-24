# reporting.oblikovati.org

The bug-report intake service for [Oblikovati](https://github.com/Oblikovati/Oblikovati).
The application's **Help ▸ Report Bug** dialog POSTs a diagnostic payload (the user's
comment, settings, open documents, platform/build, transaction log, and two screenshots)
to this service. The service is a thin shim: it validates the request, queues it in memory,
and — in the background — stores the screenshots and opens a GitHub issue that embeds them.
A reconciler deletes a report's screenshots once its issue is closed.

It is **stdlib-only** (no third-party Go dependencies).

## How it works

```
app ──POST /report──▶ ingest ──▶ in-memory queue ──▶ worker ──▶ GitHub issue
                        │                               │
                        │                               └─▶ screenshots on a volume,
                        │                                   served at /r/<id>/*.png
                        └─ Authorization: crc32(body)    reconciler ──▶ deletes them
                                                          when the issue closes
```

- The report payload is JSON; the two PNGs are base64-encoded `[]byte`. The payload shape is
  **duplicated** from the app's `report.Payload` (kept in sync by a round-trip test on each
  side) so this service stays standalone.
- Screenshots are stored under `STORAGE_DIR/<report-id>/` and served at
  `/r/<report-id>/window.png` and `/r/<report-id>/viewport.png`. The created issue embeds
  those URLs, so they render in the issue (GitHub fetches and caches them).
- An `issue.json` next to the screenshots records the issue number. The reconciler polls
  each issue's state and removes the directory once it is `closed`. Because this state lives
  on the volume (not in the in-memory queue), it is correct across restarts.
- The issue body is kept under GitHub's 65536-character create-issue limit: the comment,
  screenshots and environment table are always written in full, and the remaining budget is
  spent on documents, then the transaction log, then user settings, in that order. A
  document, transaction event or the settings block is never truncated mid-YAML — one that
  would not fit whole is left out entirely and named in a trailing note instead, so every
  block a triager sees in the issue is complete and trustworthy.
- If opening the issue fails, a network error or a GitHub 5xx is retried (up to 3 attempts,
  with backoff); a 4xx (bad request, auth, validation) fails immediately since retrying would
  just resend the same request. A report that still fails is **dead-lettered**: its full
  payload and the failure cause are written to `deadletter.json` next to its screenshots
  instead of being dropped, and the worker log names the report so it can be found and
  recreated manually (`grep dead-letter` in the container logs, then
  `cat STORAGE_DIR/<report-id>/deadletter.json`).

## Authorization

The endpoint is open, so each request must carry an `Authorization` header equal to the
**CRC-32 (IEEE) of the exact request body**, lowercase zero-padded hex (e.g. `1a2b3c4d`).
The server recomputes it over the bytes it received and rejects a mismatch with `401`. This
is a cheap probe filter to keep idle scanners out — **not** real authentication.

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/report` | Ingest a report. `202` queued, `401` bad token, `413` too large, `503` queue full. |
| `GET` | `/r/<id>/<file>` | Serve a stored screenshot. |
| `GET` | `/healthz` | Liveness probe. |

## Configuration (environment)

| Variable | Default | Notes |
|---|---|---|
| `REPORTING_GITHUB_TOKEN` | — | **Required.** GitHub PAT with `issues:write` on the target repo. |
| `REPORTING_ADDR` | `:8080` | Listen address. |
| `REPORTING_PUBLIC_BASE_URL` | `https://reporting.oblikovati.org` | Used to build screenshot URLs in issues. |
| `REPORTING_STORAGE_DIR` | `/data/reports` | Screenshot + metadata volume. |
| `REPORTING_GITHUB_OWNER` | `Oblikovati` | Issue repo owner. |
| `REPORTING_GITHUB_REPO` | `Oblikovati` | Issue repo name. |
| `REPORTING_GITHUB_API_BASE` | _(public API)_ | Override the GitHub REST base (staging/local mock). |
| `REPORTING_POLL_INTERVAL` | `15m` | Reconciler sweep interval. |
| `REPORTING_QUEUE_SIZE` | `256` | In-memory queue capacity. |
| `REPORTING_MAX_BODY_BYTES` | `26214400` | Request body cap (~25 MiB). |

## Run locally

```sh
go test ./...
REPORTING_GITHUB_TOKEN=ghp_xxx REPORTING_STORAGE_DIR=./data go run ./cmd/reportingd
```

Or with Docker:

```sh
docker build -t reportingd .
docker run --rm -p 8080:8080 -e REPORTING_GITHUB_TOKEN=ghp_xxx \
  -v "$PWD/data:/data/reports" reportingd
```

## Deployment

`.github/workflows/deploy.yml` runs on a push to `main` (or a `v*` tag): it tests, builds and
pushes the image to GHCR, then deploys over SSH (`docker compose pull && docker compose up -d`
on the host, with the image ref and secrets written to a host-side `.env`).

### Required GitHub secrets

| Secret | Purpose |
|---|---|
| `SSH_HOST`, `SSH_USERNAME`, `SSH_KEY`, `SSH_PORT` | Deploy target connection. |
| `REPORTING_GITHUB_TOKEN` | Runtime PAT the service uses to open/read issues (written to the host `.env`). |
| `REPORTING_PUBLIC_BASE_URL` | Public origin for screenshot links. |
| `REPORTING_GITHUB_OWNER`, `REPORTING_GITHUB_REPO` | Target issue repository. |

`GITHUB_TOKEN` (built-in) is used to push the image to GHCR.

> **Privacy note:** reports include user settings and document file paths. They are alpha
> diagnostics filed as GitHub issues — avoid pasting anything sensitive beyond what triage
> needs.
