# ytgui API contract (v1)

Backend: Go on `http://127.0.0.1:8765` (fallback 8766). UI is served from `/` by the same server.

## Auth and security

- Browser opens `http://127.0.0.1:<port>/?token=<hex>`. The UI must read `token` from the query string,
  store it in `sessionStorage` (key `ytgui.token`), and strip it from the URL with `history.replaceState`.
- Every `/api/*` request (except `GET /api/health`) must send header `X-Ytgui-Token: <token>`.
- `401` means the token is missing or wrong: show a "Please reopen the app from its launcher window" screen.
- The server checks `Host` and `Origin`. Use relative URLs (`/api/...`) only.
- JSON everywhere. Errors: `{"error": "plain-language message", "code": "short_code"}` with a 4xx/5xx status.

## Types (TypeScript)

```ts
type ToolName = "yt-dlp" | "ffmpeg" | "deno";

interface ToolStatus {
  name: ToolName;
  installed: string | null;     // installed version, null if missing
  previous: string | null;      // version available for rollback
  latest: string | null;        // null if not checked yet or check failed
  updateAvailable: boolean;
  busy: boolean;                // an install/update is running
  required: boolean;            // yt-dlp, ffmpeg: true; deno: false (recommended)
  error?: string;               // last check/update error
}

interface ToolsResponse {
  tools: ToolStatus[];
  ready: boolean;               // all required tools installed
  appVersion: string;
}

interface QualityOption {
  id: string;                   // "best" | "2160" | "1440" | "1080" | "720" | "480" | "360" | "240" | "144" | "audio"
  label: string;                // "1080p", "Best available", "Audio only (m4a)"
  height: number | null;
  approxBytes: number | null;   // estimated total size, may be null
}

interface VideoInfo {
  url: string;                  // canonical webpage URL
  id: string;
  title: string;
  uploader: string | null;
  durationSec: number | null;
  thumbnail: string | null;     // remote URL, show with <img referrerpolicy="no-referrer">
  qualities: QualityOption[];   // highest first, then "audio" last
}

type JobState = "queued" | "running" | "done" | "failed" | "canceled";

interface Job {
  id: string;
  url: string;
  title: string;
  thumbnail: string | null;
  quality: string;              // QualityOption.id
  state: JobState;
  phase: string;                // "Waiting" | "Downloading video" | "Downloading audio" | "Merging" | "Finished" | ...
  percent: number;              // 0-100 for the current phase
  speed: number | null;         // bytes/sec
  eta: number | null;           // seconds
  attempt: number;              // 1..3
  error: string | null;         // plain-language reason when failed
  suggestUpdate: boolean;       // failure looks like a stale yt-dlp
  filePath: string | null;      // final file when done
  createdAt: string;            // ISO time
  finishedAt: string | null;
}

interface JobsResponse { jobs: Job[]; queuePaused: boolean; }

interface Settings {
  downloadDir: string;
  defaultQuality: string;       // QualityOption.id, default "1080"
}

interface HistoryEntry {
  id: string; url: string; title: string; quality: string;
  filePath: string | null; state: "done" | "failed"; error: string | null; finishedAt: string;
}
```

## Routes

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/api/health` | – | `{"app":"ytgui","version":"..."}` (no token needed) |
| GET | `/api/tools` | – | `ToolsResponse` |
| POST | `/api/tools/check` | – | `ToolsResponse` (refresh `latest` from the internet) |
| POST | `/api/tools/setup` | – | `202`; installs every missing tool (first-run). Progress over SSE |
| POST | `/api/tools/{name}/update` | – | `202`; progress over SSE |
| POST | `/api/tools/{name}/rollback` | – | `ToolsResponse` |
| POST | `/api/info` | `{"url": string}` | `VideoInfo` (can take 5–30 s) |
| GET | `/api/jobs` | – | `JobsResponse` |
| POST | `/api/jobs` | `{"url": string, "quality": string, "title"?: string, "thumbnail"?: string}` | `Job` |
| DELETE | `/api/jobs/{id}` | – | `204` cancel if active, remove if finished |
| POST | `/api/jobs/{id}/retry` | – | `Job` (re-queue a failed/canceled job) |
| POST | `/api/queue/resume` | – | `JobsResponse` (queue pauses after a job fails) |
| POST | `/api/jobs/clear` | – | `JobsResponse` (removes finished/failed/canceled from the list) |
| GET | `/api/settings` | – | `Settings` |
| PUT | `/api/settings` | `Settings` | `Settings` |
| GET | `/api/history` | – | `HistoryEntry[]` (newest first) |
| DELETE | `/api/history` | – | `204` |
| POST | `/api/open-folder` | `{"path"?: string}` | `204`; no path = downloads folder; a file path reveals that file |
| POST | `/api/pick-folder` | – | `{"path": string | null}` native folder picker (null if canceled) |
| POST | `/api/quit` | – | `204`, then the server exits |
| GET | `/api/events` | – | SSE stream (see below) |

## SSE: `GET /api/events`

`EventSource` cannot send headers, so the UI must use `fetch()` with the token header and parse the
`text/event-stream` body itself (lines `event: <name>`, `data: <json>`, blank line ends an event).
Reconnect with backoff if the stream ends. If reconnects fail for ~3 s after a Quit or a crash, show
an "App stopped. You can close this tab." screen.

Events:

| event | data |
|---|---|
| `hello` | `{"jobs": JobsResponse, "tools": ToolsResponse}` sent on connect (full snapshot) |
| `job` | `Job` (full object, on every change; progress throttled to ~4/s) |
| `job-removed` | `{"id": string}` |
| `queue` | `{"queuePaused": boolean}` |
| `tools` | `ToolsResponse` |
| `tool-progress` | `{"name": ToolName, "stage": "checking"|"downloading"|"verifying"|"testing"|"installing"|"done"|"error", "percent": number, "message": string}` |
| `ping` | `{}` every 15 s |
