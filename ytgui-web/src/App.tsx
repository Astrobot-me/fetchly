import { useEffect, useState } from "react";
import { api, getToken } from "./api/client";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { UpdateBanner } from "./components/UpdateBanner";
import { BadToken } from "./pages/BadToken";
import { Home } from "./pages/Home";
import { Settings } from "./pages/Settings";
import { Setup } from "./pages/Setup";
import { Stopped } from "./pages/Stopped";
import { actions, startStore, useStore } from "./state/store";

type Tab = "home" | "settings";

function tabFromHash(): Tab {
  return window.location.hash === "#settings" ? "settings" : "home";
}

export function App() {
  const s = useStore();
  const [tab, setTab] = useState<Tab>(tabFromHash);
  const [quitOpen, setQuitOpen] = useState(false);
  const [quitting, setQuitting] = useState(false);
  // Stay on the setup screen until optional tools finish too, once it has been shown.
  const [inSetup, setInSetup] = useState(false);

  useEffect(() => {
    if (!getToken()) {
      actions.setBadToken();
      return;
    }
    startStore();
  }, []);

  useEffect(() => {
    const onHash = () => setTab(tabFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const tools = s.tools;
  const anyToolBusy = !!tools?.tools.some((t) => t.busy);
  useEffect(() => {
    if (!tools) return;
    if (!tools.ready) setInSetup(true);
    else if (!anyToolBusy) setInSetup(false);
  }, [tools, anyToolBusy]);

  const go = (t: Tab) => {
    setTab(t);
    const hash = t === "settings" ? "#settings" : "";
    window.history.replaceState(window.history.state, "", window.location.pathname + window.location.search + hash);
    window.scrollTo({ top: 0 });
  };

  const quit = async () => {
    setQuitting(true);
    try {
      await api.quit();
    } catch {
      /* the server may close the connection while exiting */
    }
    actions.setStopped();
  };

  if (s.badToken) return <BadToken />;
  if (s.stopped) return <Stopped />;

  const showSetup = !!tools && (!tools.ready || inSetup);

  return (
    <div className="app">
      <header className="topbar">
        <div className="topbar__inner">
          <a
            className="brand"
            href="#"
            onClick={(e) => {
              e.preventDefault();
              go("home");
            }}
          >
            <span className="brand__mark" aria-hidden="true">
              <svg viewBox="0 0 32 32" width="30" height="30">
                <rect width="32" height="32" rx="10" fill="currentColor" />
                <path
                  d="M16 8v12m-5.5-5.5L16 20l5.5-5.5M10 25h12"
                  fill="none"
                  stroke="var(--sun)"
                  strokeWidth="3"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            </span>
            <span className="brand__name">Fetchly</span>
          </a>

          {!showSetup && s.hydrated && (
            <nav className="tabs" aria-label="Main">
              <button
                type="button"
                className={`tabs__btn${tab === "home" ? " is-on" : ""}`}
                aria-current={tab === "home" ? "page" : undefined}
                onClick={() => go("home")}
              >
                Download
              </button>
              <button
                type="button"
                className={`tabs__btn${tab === "settings" ? " is-on" : ""}`}
                aria-current={tab === "settings" ? "page" : undefined}
                onClick={() => go("settings")}
              >
                Settings
              </button>
            </nav>
          )}

          <button type="button" className="btn btn--quit" onClick={() => setQuitOpen(true)}>
            <svg aria-hidden="true" viewBox="0 0 24 24" width="16" height="16">
              <path
                d="M12 3v8M7.05 6.05a7 7 0 1 0 9.9 0"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.2"
                strokeLinecap="round"
              />
            </svg>
            Quit
          </button>
        </div>
      </header>

      {s.connection === "reconnecting" && s.hydrated && (
        <div className="reconnect" role="status">
          Reconnecting…
        </div>
      )}

      <main className="main">
        {!s.hydrated ? (
          <div className="loading" role="status">
            <span className="spinner" aria-hidden="true" />
            <p>Starting up…</p>
          </div>
        ) : showSetup ? (
          <Setup />
        ) : (
          <>
            <UpdateBanner tools={tools} onOpenSettings={() => go("settings")} />
            {tab === "home" ? <Home /> : <Settings />}
          </>
        )}
      </main>

      <ConfirmDialog
        open={quitOpen}
        title="Quit Fetchly?"
        body={
          s.jobs.some((j) => j.state === "running" || j.state === "queued")
            ? "Downloads in progress will stop. You can start them again next time."
            : "Fetchly will close. You can open it again any time."
        }
        confirmLabel={quitting ? "Quitting…" : "Quit"}
        cancelLabel="Keep using"
        danger
        busy={quitting}
        onConfirm={quit}
        onCancel={() => setQuitOpen(false)}
      />
    </div>
  );
}
