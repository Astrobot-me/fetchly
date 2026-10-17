// Types mirror API.md (v1) exactly.

export type ToolName = "yt-dlp" | "ffmpeg" | "deno";

export interface ToolStatus {
  name: ToolName;
  installed: string | null;
  previous: string | null;
  latest: string | null;
  updateAvailable: boolean;
  busy: boolean;
  required: boolean;
  error?: string;
}

export interface ToolsResponse {
  tools: ToolStatus[];
  ready: boolean;
  appVersion: string;
}

export interface QualityOption {
  id: string;
  label: string;
  height: number | null;
  approxBytes: number | null;
}

export interface VideoInfo {
  url: string;
  id: string;
  title: string;
  uploader: string | null;
  durationSec: number | null;
  thumbnail: string | null;
  qualities: QualityOption[];
}

export type JobState = "queued" | "running" | "done" | "failed" | "canceled";

export interface Job {
  id: string;
  url: string;
  title: string;
  thumbnail: string | null;
  quality: string;
  state: JobState;
  phase: string;
  percent: number;
  speed: number | null;
  eta: number | null;
  attempt: number;
  error: string | null;
  suggestUpdate: boolean;
  filePath: string | null;
  createdAt: string;
  finishedAt: string | null;
}

export interface JobsResponse {
  jobs: Job[];
  queuePaused: boolean;
}

export interface Settings {
  downloadDir: string;
  defaultQuality: string;
}

export interface HistoryEntry {
  id: string;
  url: string;
  title: string;
  quality: string;
  filePath: string | null;
  state: "done" | "failed";
  error: string | null;
  finishedAt: string;
}

export interface HealthResponse {
  app: string;
  version: string;
}

export interface CreateJobRequest {
  url: string;
  quality: string;
  title?: string;
  thumbnail?: string;
}

export interface PickFolderResponse {
  path: string | null;
}

export interface ApiErrorBody {
  error: string;
  code: string;
}

// ---- SSE payloads ----

export type ToolStage =
  | "checking"
  | "downloading"
  | "verifying"
  | "testing"
  | "installing"
  | "done"
  | "error";

export interface ToolProgress {
  name: ToolName;
  stage: ToolStage;
  percent: number;
  message: string;
}

export interface HelloEvent {
  jobs: JobsResponse;
  tools: ToolsResponse;
}

export interface JobRemovedEvent {
  id: string;
}

export interface QueueEvent {
  queuePaused: boolean;
}

export type ServerEvent =
  | { event: "hello"; data: HelloEvent }
  | { event: "job"; data: Job }
  | { event: "job-removed"; data: JobRemovedEvent }
  | { event: "queue"; data: QueueEvent }
  | { event: "tools"; data: ToolsResponse }
  | { event: "tool-progress"; data: ToolProgress }
  | { event: "ping"; data: Record<string, never> };
