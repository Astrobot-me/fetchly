import type {
  ApiErrorBody,
  CreateJobRequest,
  HistoryEntry,
  Job,
  JobsResponse,
  PickFolderResponse,
  Settings,
  ToolName,
  ToolsResponse,
  VideoInfo,
} from "./types";

const TOKEN_KEY = "ytgui.token";
const TOKEN_HEADER = "X-Ytgui-Token";

let memoryToken: string | null = null;

/**
 * Reads `?token=` from the URL, stores it in sessionStorage and strips it from
 * the address bar. Returns the token to use (from the URL or a previous load).
 */
export function initToken(): string | null {
  const url = new URL(window.location.href);
  const fromUrl = url.searchParams.get("token");
  if (fromUrl) {
    memoryToken = fromUrl;
    try {
      sessionStorage.setItem(TOKEN_KEY, fromUrl);
    } catch {
      /* storage blocked: keep it in memory only */
    }
    url.searchParams.delete("token");
    const query = url.searchParams.toString();
    window.history.replaceState(
      window.history.state,
      "",
      url.pathname + (query ? `?${query}` : "") + url.hash,
    );
  }
  return getToken();
}

export function getToken(): string | null {
  if (memoryToken) return memoryToken;
  try {
    memoryToken = sessionStorage.getItem(TOKEN_KEY);
  } catch {
    memoryToken = null;
  }
  return memoryToken;
}

export function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { [TOKEN_HEADER]: token } : {};
}

// ---- 401 notification ----

type Listener = () => void;
const unauthorizedListeners = new Set<Listener>();

export function onUnauthorized(fn: Listener): () => void {
  unauthorizedListeners.add(fn);
  return () => unauthorizedListeners.delete(fn);
}

export function notifyUnauthorized(): void {
  unauthorizedListeners.forEach((fn) => fn());
}

// ---- errors ----

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(message: string, status: number, code: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

/** Plain-language message for any thrown value. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error && err.message) return err.message;
  return "Something went wrong. Please try again.";
}

function isErrorBody(v: unknown): v is ApiErrorBody {
  return (
    typeof v === "object" &&
    v !== null &&
    typeof (v as { error?: unknown }).error === "string"
  );
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json", ...authHeaders() };
  if (body !== undefined) headers["Content-Type"] = "application/json";

  let res: Response;
  try {
    res = await fetch(path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
    });
  } catch {
    throw new ApiError(
      "Can't reach Fetchly. It may have stopped running.",
      0,
      "network",
    );
  }

  if (res.status === 401) {
    notifyUnauthorized();
    throw new ApiError("Please reopen the app from its launcher window.", 401, "unauthorized");
  }

  if (!res.ok) {
    let parsed: unknown = null;
    try {
      parsed = await res.json();
    } catch {
      /* not JSON */
    }
    if (isErrorBody(parsed)) {
      throw new ApiError(parsed.error, res.status, parsed.code || "error");
    }
    throw new ApiError(
      `Something went wrong (error ${res.status}). Please try again.`,
      res.status,
      "http_" + res.status,
    );
  }

  if (res.status === 204 || res.status === 202) {
    return undefined as T;
  }
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

const enc = encodeURIComponent;

export const api = {
  tools: () => request<ToolsResponse>("GET", "/api/tools"),
  checkTools: () => request<ToolsResponse>("POST", "/api/tools/check"),
  setupTools: () => request<void>("POST", "/api/tools/setup"),
  updateTool: (name: ToolName) => request<void>("POST", `/api/tools/${enc(name)}/update`),
  rollbackTool: (name: ToolName) =>
    request<ToolsResponse>("POST", `/api/tools/${enc(name)}/rollback`),

  info: (url: string) => request<VideoInfo>("POST", "/api/info", { url }),

  jobs: () => request<JobsResponse>("GET", "/api/jobs"),
  createJob: (req: CreateJobRequest) => request<Job>("POST", "/api/jobs", req),
  deleteJob: (id: string) => request<void>("DELETE", `/api/jobs/${enc(id)}`),
  retryJob: (id: string) => request<Job>("POST", `/api/jobs/${enc(id)}/retry`),
  resumeQueue: () => request<JobsResponse>("POST", "/api/queue/resume"),
  clearJobs: () => request<JobsResponse>("POST", "/api/jobs/clear"),

  settings: () => request<Settings>("GET", "/api/settings"),
  saveSettings: (s: Settings) => request<Settings>("PUT", "/api/settings", s),

  history: () => request<HistoryEntry[]>("GET", "/api/history"),
  clearHistory: () => request<void>("DELETE", "/api/history"),

  openFolder: (path?: string) =>
    request<void>("POST", "/api/open-folder", path ? { path } : {}),
  pickFolder: () => request<PickFolderResponse>("POST", "/api/pick-folder"),

  quit: () => request<void>("POST", "/api/quit"),
};
