import { useSyncExternalStore } from "react";
import { api, onUnauthorized } from "../api/client";
import { connectEvents } from "../api/events";
import type {
  HelloEvent,
  Job,
  JobRemovedEvent,
  QueueEvent,
  Settings,
  ToolName,
  ToolProgress,
  ToolsResponse,
} from "../api/types";

export type Connection = "connecting" | "open" | "reconnecting";

export interface AppState {
  connection: Connection;
  /** Got the first `hello` snapshot. */
  hydrated: boolean;
  /** Server is gone (quit or crash). */
  stopped: boolean;
  /** 401: token missing or wrong. */
  badToken: boolean;
  jobs: Job[];
  queuePaused: boolean;
  tools: ToolsResponse | null;
  toolProgress: Partial<Record<ToolName, ToolProgress>>;
  settings: Settings | null;
}

let state: AppState = {
  connection: "connecting",
  hydrated: false,
  stopped: false,
  badToken: false,
  jobs: [],
  queuePaused: false,
  tools: null,
  toolProgress: {},
  settings: null,
};

const listeners = new Set<() => void>();

function set(patch: Partial<AppState>): void {
  state = { ...state, ...patch };
  listeners.forEach((l) => l());
}

function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function getState(): AppState {
  return state;
}

export function useStore(): AppState {
  return useSyncExternalStore(subscribe, getState, getState);
}

// ---- mutations (also used after REST responses so the UI updates instantly) ----

function sortJobs(jobs: Job[]): Job[] {
  // Newest first, like a browser's download list.
  return [...jobs].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

export const actions = {
  setJobs(jobs: Job[], queuePaused: boolean) {
    set({ jobs: sortJobs(jobs), queuePaused });
  },
  upsertJob(job: Job) {
    const others = state.jobs.filter((j) => j.id !== job.id);
    set({ jobs: sortJobs([...others, job]) });
  },
  removeJob(id: string) {
    set({ jobs: state.jobs.filter((j) => j.id !== id) });
  },
  setQueuePaused(queuePaused: boolean) {
    set({ queuePaused });
  },
  setTools(tools: ToolsResponse) {
    set({ tools });
  },
  setToolProgress(p: ToolProgress) {
    set({ toolProgress: { ...state.toolProgress, [p.name]: p } });
  },
  setSettings(settings: Settings) {
    set({ settings });
  },
  setStopped() {
    set({ stopped: true });
  },
  setBadToken() {
    set({ badToken: true });
  },
};

// ---- SSE wiring ----

function handleEvent(name: string, data: unknown): void {
  switch (name) {
    case "hello": {
      const d = data as HelloEvent;
      set({
        jobs: sortJobs(d.jobs.jobs),
        queuePaused: d.jobs.queuePaused,
        tools: d.tools,
        hydrated: true,
        stopped: false,
      });
      break;
    }
    case "job":
      actions.upsertJob(data as Job);
      break;
    case "job-removed":
      actions.removeJob((data as JobRemovedEvent).id);
      break;
    case "queue":
      actions.setQueuePaused((data as QueueEvent).queuePaused);
      break;
    case "tools":
      actions.setTools(data as ToolsResponse);
      break;
    case "tool-progress":
      actions.setToolProgress(data as ToolProgress);
      break;
    default:
      // "ping" and unknown events: nothing to do.
      break;
  }
}

let started = false;

/** Starts the event stream once for the lifetime of the page. */
export function startStore(): void {
  if (started) return;
  started = true;

  onUnauthorized(() => actions.setBadToken());

  connectEvents({
    onOpen: () => {
      set({ connection: "open", stopped: false });
      if (!state.settings) {
        api.settings().then(actions.setSettings, () => {
          /* Home falls back to "best"; Settings page retries on its own. */
        });
      }
    },
    onEvent: handleEvent,
    onReconnecting: () => set({ connection: "reconnecting" }),
    onDown: () => actions.setStopped(),
    onUnauthorized: () => actions.setBadToken(),
  });
}
