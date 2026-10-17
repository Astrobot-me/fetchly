import { authHeaders } from "./client";

/**
 * Fetch-based Server-Sent Events client for GET /api/events.
 * EventSource cannot send the X-Ytgui-Token header, so we read and parse the
 * text/event-stream body ourselves and reconnect with backoff.
 */

export interface EventHandlers {
  /** Stream connected (a `hello` snapshot follows). */
  onOpen(): void;
  /** One parsed event. `data` is the JSON-decoded payload. */
  onEvent(name: string, data: unknown): void;
  /** Stream dropped; we are trying to reconnect. */
  onReconnecting(): void;
  /** Reconnects have been failing for ~3 s: the app has stopped. */
  onDown(): void;
  /** Server answered 401: token is missing or wrong. Stream stops. */
  onUnauthorized(): void;
}

const DOWN_AFTER_MS = 3000;
const CONNECT_TIMEOUT_MS = 5000;
// Server pings every 15 s; if nothing arrives for this long, assume a dead stream.
const IDLE_TIMEOUT_MS = 40000;

export function connectEvents(h: EventHandlers): () => void {
  let closed = false;
  let controller: AbortController | null = null;
  let retryTimer: number | undefined;
  let failingSince: number | null = null;
  let attempt = 0;
  let downReported = false;

  const scheduleReconnect = () => {
    if (closed) return;
    const now = Date.now();
    if (failingSince === null) {
      failingSince = now;
      h.onReconnecting();
    }
    if (!downReported && now - failingSince >= DOWN_AFTER_MS) {
      downReported = true;
      h.onDown();
    }
    // 250, 500, 1000, 1000... while deciding; slower once we know it's down.
    const delay = downReported ? 5000 : Math.min(250 * 2 ** attempt, 1000);
    attempt++;
    retryTimer = window.setTimeout(run, delay);
  };

  const run = async () => {
    if (closed) return;
    const ctrl = new AbortController();
    controller = ctrl;
    let watchdog: number | undefined;
    const arm = (ms: number) => {
      window.clearTimeout(watchdog);
      watchdog = window.setTimeout(() => ctrl.abort(), ms);
    };

    arm(CONNECT_TIMEOUT_MS);
    try {
      const res = await fetch("/api/events", {
        headers: { Accept: "text/event-stream", ...authHeaders() },
        cache: "no-store",
        signal: ctrl.signal,
      });
      if (res.status === 401) {
        window.clearTimeout(watchdog);
        closed = true;
        h.onUnauthorized();
        return;
      }
      if (!res.ok || !res.body) throw new Error(`events: HTTP ${res.status}`);

      failingSince = null;
      attempt = 0;
      downReported = false;
      arm(IDLE_TIMEOUT_MS);
      h.onOpen();

      await readEventStream(res.body, () => arm(IDLE_TIMEOUT_MS), h.onEvent);
    } catch {
      /* network error, abort or bad status: fall through to reconnect */
    } finally {
      window.clearTimeout(watchdog);
    }
    scheduleReconnect();
  };

  void run();

  return () => {
    closed = true;
    window.clearTimeout(retryTimer);
    controller?.abort();
  };
}

/** Parses a text/event-stream body per the SSE spec (event/data fields, blank line dispatch). */
async function readEventStream(
  body: ReadableStream<Uint8Array>,
  onActivity: () => void,
  dispatch: (name: string, data: unknown) => void,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let eventName = "";
  let dataLines: string[] = [];

  const flush = () => {
    if (dataLines.length === 0) {
      eventName = "";
      return;
    }
    const raw = dataLines.join("\n");
    const name = eventName || "message";
    eventName = "";
    dataLines = [];
    let data: unknown;
    try {
      data = JSON.parse(raw);
    } catch {
      return; // ignore malformed payloads
    }
    dispatch(name, data);
  };

  const handleLine = (line: string) => {
    if (line === "") {
      flush();
      return;
    }
    if (line.startsWith(":")) return; // comment
    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field === "event") eventName = value;
    else if (field === "data") dataLines.push(value);
    // "id" and "retry" are not used by this API.
  };

  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    onActivity();
    buffer += decoder.decode(value, { stream: true });
    let nl: number;
    while ((nl = buffer.indexOf("\n")) !== -1) {
      let line = buffer.slice(0, nl);
      buffer = buffer.slice(nl + 1);
      if (line.endsWith("\r")) line = line.slice(0, -1);
      handleLine(line);
    }
  }
}
