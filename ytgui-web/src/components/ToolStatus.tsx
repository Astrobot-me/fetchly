import { useState } from "react";
import { api, errorMessage } from "../api/client";
import type { ToolProgress, ToolStatus as Tool } from "../api/types";
import { TOOL_INFO } from "../format";
import { actions } from "../state/store";
import { ProgressBar } from "./ProgressBar";

const STAGE_TEXT: Record<ToolProgress["stage"], string> = {
  checking: "Checking",
  downloading: "Downloading",
  verifying: "Checking the download",
  testing: "Testing",
  installing: "Installing",
  done: "Done",
  error: "Problem",
};

interface Props {
  tool: Tool;
  progress?: ToolProgress;
  /** "setup" = simple first-run row; "settings" = versions and actions. */
  variant: "setup" | "settings";
}

export function ToolStatus({ tool, progress, variant }: Props) {
  const info = TOOL_INFO[tool.name];
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const running = tool.busy || busy;
  const showProgress = tool.busy && progress && progress.stage !== "done" && progress.stage !== "error";
  const progressError = progress?.stage === "error" ? progress.message : null;
  const problem = err ?? progressError ?? tool.error ?? null;

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  let status: string;
  if (showProgress) status = progress.message || STAGE_TEXT[progress.stage];
  else if (tool.busy) status = "Working…";
  else if (tool.installed) status = variant === "setup" ? "Ready" : `Version ${tool.installed}`;
  else status = variant === "setup" ? "Waiting" : "Not installed";

  const state = tool.installed && !tool.busy ? "ok" : problem && !tool.busy ? "bad" : tool.busy ? "busy" : "idle";

  return (
    <li className={`tool tool--${state}`}>
      <span className="tool__mark" aria-hidden="true">
        {state === "ok" ? (
          <svg viewBox="0 0 24 24" width="16" height="16">
            <path d="M5 12.5l4.5 4.5L19 7.5" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        ) : state === "bad" ? (
          "!"
        ) : null}
      </span>
      <div className="tool__body">
        <div className="tool__head">
          <span className="tool__name">{info.friendly}</span>
          <span className="tool__tech">{tool.name}</span>
          {!tool.required && <span className="tag">Optional</span>}
        </div>
        <p className="tool__what">{tool.required ? info.what : "Recommended – improves YouTube support"}</p>
        <p className="tool__status">{status}</p>

        {variant === "settings" && (
          <dl className="tool__versions">
            <div>
              <dt>Installed</dt>
              <dd>{tool.installed ?? "—"}</dd>
            </div>
            <div>
              <dt>Latest</dt>
              <dd>{tool.latest ?? "not checked"}</dd>
            </div>
            {tool.previous && (
              <div>
                <dt>Previous</dt>
                <dd>{tool.previous}</dd>
              </div>
            )}
          </dl>
        )}

        {showProgress && (
          <ProgressBar
            percent={progress.percent > 0 ? progress.percent : null}
            label={`${info.friendly}: ${STAGE_TEXT[progress.stage]}`}
            size="sm"
          />
        )}
        {problem && !tool.busy && <p className="tool__error">{problem}</p>}

        {variant === "settings" && (
          <div className="tool__actions">
            {(tool.updateAvailable || !tool.installed) && (
              <button
                type="button"
                className="btn btn--primary btn--sm"
                disabled={running}
                onClick={() => run(() => api.updateTool(tool.name))}
              >
                {tool.installed ? `Update to ${tool.latest ?? "latest"}` : "Install"}
              </button>
            )}
            {tool.previous && (
              <button
                type="button"
                className="btn btn--ghost btn--sm"
                disabled={running}
                onClick={() => run(async () => actions.setTools(await api.rollbackTool(tool.name)))}
              >
                Go back to {tool.previous}
              </button>
            )}
          </div>
        )}
      </div>
    </li>
  );
}
