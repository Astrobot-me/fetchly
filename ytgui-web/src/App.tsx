import { useEffect, useState } from "react";
import { api, getToken } from "./api/client";
import { Backdrop } from "./components/Backdrop";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { Bolt, DownloadIcon, Gear, Heart, Power } from "./components/Icons";
import { ThemeToggle } from "./components/ThemeToggle";
import { UpdateBanner } from "./components/UpdateBanner";
import { BadToken } from "./pages/BadToken";
import { Credits } from "./pages/Credits";
import { Home } from "./pages/Home";
import { Settings } from "./pages/Settings";
import { Setup } from "./pages/Setup";
import { Stopped } from "./pages/Stopped";
import { actions, startStore, useStore } from "./state/store";

type Tab = "home" | "settings" | "credits";

const TABS: { id: Tab; label: string; hash: string; Icon: typeof Gear }[] = [
  { id: "home", label: "Download", hash: "", Icon: DownloadIcon },
  { id: "settings", label: "Settings", hash: "#settings", Icon: Gear },
  { id: "credits", label: "Credits", hash: "#credits", Icon: Heart },
];

function tabFromHash(): Tab {
  const h = window.location.hash;
  if (h === "#settings") return "settings";
  if (h === "#credits") return "credits";
  return "home";
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
    const hash = TABS.find((x) => x.id === t)?.hash ?? "";
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

  if (s.badToken)
    return (
      <>
        <Backdrop />
        <BadToken />
      </>
    );
  if (s.stopped)
    return (
      <>
        <Backdrop />
        <Stopped />
      </>
    );

  const showSetup = !!tools && (!tools.ready || inSetup);
  const showTabs = !showSetup && s.hydrated;

  return (
    <div className="app">
      <Backdrop />
      <header className="topbar">
        <nav className="nav" aria-label="Main">
          <a
            className="brand"
            href="#"
            onClick={(e) => {
              e.preventDefault();
              go("home");
            }}
          >
            <span className="brand__mark">
              <Bolt size={24} />
            </span>
            <span className="brand__name">
              Fetch<span className="brand__ly">ly</span>
            </span>
          </a>

          {showTabs && (
            <div className="nav__tabs">
              {TABS.map(({ id, label, Icon }) => (
                <button
                  key={id}
                  type="button"
                  className={`nav__tab${tab === id ? " is-on" : ""}`}
                  aria-current={tab === id ? "page" : undefined}
                  onClick={() => go(id)}
                >
                  <Icon size={18} />
                  <span className="nav__label">{label}</span>
                </button>
              ))}
            </div>
          )}

          <span className="nav__sep" aria-hidden="true" />
          <ThemeToggle />
          <button
            type="button"
            className="navbtn navbtn--quit"
            onClick={() => setQuitOpen(true)}
            aria-label="Quit Fetchly"
            title="Quit Fetchly"
          >
            <Power size={18} />
          </button>
        </nav>
      </header>

      {s.connection === "reconnecting" && s.hydrated && (
        <div className="reconnect" role="status">
          <span className="spinner spinner--sm" aria-hidden="true" />
          Reconnecting…
        </div>
      )}

      <main className={`main main--${showSetup ? "setup" : tab}`}>
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
            {tab === "home" ? <Home /> : tab === "settings" ? <Settings /> : <Credits />}
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
