import { useState } from "react";
import type { ToolsResponse } from "../api/types";

const KEY = "fetchly.updateBannerDismissed";

function readDismissed(): string | null {
  try {
    return sessionStorage.getItem(KEY);
  } catch {
    return null;
  }
}

interface Props {
  tools: ToolsResponse | null;
  onOpenSettings: () => void;
}

/** Slim, dismissible notice when any helper has an update. Never updates on its own. */
export function UpdateBanner({ tools, onOpenSettings }: Props) {
  const pending = tools?.tools.filter((t) => t.updateAvailable) ?? [];
  // Signature of what is pending, so a newer update shows the banner again.
  const sig = pending.map((t) => `${t.name}@${t.latest}`).join(",");
  const [dismissed, setDismissed] = useState<string | null>(readDismissed);

  if (pending.length === 0 || dismissed === sig) return null;

  return (
    <div className="banner banner--info" role="status">
      <span className="banner__dot" aria-hidden="true" />
      <p className="banner__text">An update for the download tools is ready.</p>
      <button type="button" className="btn btn--link" onClick={onOpenSettings}>
        See updates
      </button>
      <button
        type="button"
        className="iconbtn"
        aria-label="Dismiss"
        onClick={() => {
          setDismissed(sig);
          try {
            sessionStorage.setItem(KEY, sig);
          } catch {
            /* ignore */
          }
        }}
      >
        <svg aria-hidden="true" viewBox="0 0 24 24" width="16" height="16">
          <path d="M6 6l12 12M18 6L6 18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
        </svg>
      </button>
    </div>
  );
}
