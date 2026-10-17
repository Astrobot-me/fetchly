import { useEffect, useState } from "react";
import { api, errorMessage } from "../api/client";
import type { HistoryEntry, Settings as SettingsT } from "../api/types";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { ToolStatus } from "../components/ToolStatus";
import { QUALITY_CHOICES, formatDate, qualityLabel } from "../format";
import { actions, useStore } from "../state/store";

export function Settings() {
  return (
    <div className="page page--settings">
      <header className="page-head">
        <h1 className="page-title">Settings</h1>
        <p className="page-sub">Where videos go, which quality to pick first, and the tools behind the scenes.</p>
      </header>
      <DownloadSettings />
      <ToolsSection />
      <HistorySection />
    </div>
  );
}

function DownloadSettings() {
  const { settings } = useStore();
  const [draft, setDraft] = useState<SettingsT | null>(settings);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    api.settings().then(
      (s) => {
        actions.setSettings(s);
        setDraft(s);
      },
      (e) => setLoadErr(errorMessage(e)),
    );
  }, []);

  const dirty = !!draft && !!settings && (draft.downloadDir !== settings.downloadDir || draft.defaultQuality !== settings.defaultQuality);

  const choose = async () => {
    setMsg(null);
    try {
      const r = await api.pickFolder();
      if (r.path && draft) setDraft({ ...draft, downloadDir: r.path });
    } catch (e) {
      setMsg({ ok: false, text: errorMessage(e) });
    }
  };

  const open = async () => {
    setMsg(null);
    try {
      await api.openFolder();
    } catch (e) {
      setMsg({ ok: false, text: errorMessage(e) });
    }
  };

  const save = async () => {
    if (!draft) return;
    setSaving(true);
    setMsg(null);
    try {
      const s = await api.saveSettings(draft);
      actions.setSettings(s);
      setDraft(s);
      setMsg({ ok: true, text: "Settings saved." });
    } catch (e) {
      setMsg({ ok: false, text: errorMessage(e) });
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="card section" aria-labelledby="dl-title">
      <h2 id="dl-title" className="section-title">
        Downloads
      </h2>
      {!draft ? (
        loadErr ? <p className="form-error">{loadErr}</p> : <p className="muted">Loading…</p>
      ) : (
        <form
          className="form"
          onSubmit={(e) => {
            e.preventDefault();
            void save();
          }}
        >
          <div className="field">
            <label htmlFor="dir" className="field__label">
              Save videos to
            </label>
            <div className="field__row">
              <input
                id="dir"
                className="input"
                type="text"
                spellCheck={false}
                value={draft.downloadDir}
                onChange={(e) => setDraft({ ...draft, downloadDir: e.target.value })}
              />
              <button type="button" className="btn btn--soft" onClick={choose}>
                Choose…
              </button>
              <button type="button" className="btn btn--ghost" onClick={open}>
                Open
              </button>
            </div>
          </div>

          <div className="field">
            <label htmlFor="q" className="field__label">
              Preferred quality
            </label>
            <select
              id="q"
              className="input select"
              value={draft.defaultQuality}
              onChange={(e) => setDraft({ ...draft, defaultQuality: e.target.value })}
            >
              {QUALITY_CHOICES.map((q) => (
                <option key={q.id} value={q.id}>
                  {q.label}
                </option>
              ))}
            </select>
            <p className="field__hint">If a video doesn't have this quality, the next lower one is picked.</p>
          </div>

          <div className="form__actions">
            <button type="submit" className="btn btn--primary" disabled={saving || !dirty}>
              {saving ? "Saving…" : "Save"}
            </button>
            {msg && <p className={msg.ok ? "form-ok" : "form-error"}>{msg.text}</p>}
          </div>
        </form>
      )}
    </section>
  );
}

function ToolsSection() {
  const { tools, toolProgress } = useStore();
  const [checking, setChecking] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const check = async () => {
    setChecking(true);
    setErr(null);
    try {
      actions.setTools(await api.checkTools());
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setChecking(false);
    }
  };

  return (
    <section className="card section" aria-labelledby="tools-title">
      <div className="section__head">
        <div>
          <h2 id="tools-title" className="section-title">
            Download tools
          </h2>
          <p className="muted">Advanced. Fetchly uses these free programs behind the scenes.</p>
        </div>
        <button type="button" className="btn btn--soft btn--sm" onClick={check} disabled={checking}>
          {checking ? "Checking…" : "Check for updates"}
        </button>
      </div>
      {err && <p className="form-error">{err}</p>}
      <ul className="toollist">
        {(tools?.tools ?? []).map((t) => (
          <ToolStatus key={t.name} tool={t} progress={toolProgress[t.name]} variant="settings" />
        ))}
      </ul>
    </section>
  );
}

function HistorySection() {
  const [items, setItems] = useState<HistoryEntry[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [confirm, setConfirm] = useState(false);
  const [clearing, setClearing] = useState(false);

  useEffect(() => {
    api.history().then(setItems, (e) => setErr(errorMessage(e)));
  }, []);

  const clear = async () => {
    setClearing(true);
    setErr(null);
    try {
      await api.clearHistory();
      setItems([]);
      setConfirm(false);
    } catch (e) {
      setErr(errorMessage(e));
      setConfirm(false);
    } finally {
      setClearing(false);
    }
  };

  return (
    <section className="card section" aria-labelledby="hist-title">
      <div className="section__head">
        <h2 id="hist-title" className="section-title">
          History
        </h2>
        {items && items.length > 0 && (
          <button type="button" className="btn btn--ghost btn--sm" onClick={() => setConfirm(true)}>
            Clear history
          </button>
        )}
      </div>
      {err && <p className="form-error">{err}</p>}
      {items === null && !err && <p className="muted">Loading…</p>}
      {items && items.length === 0 && <p className="muted">No past downloads yet.</p>}
      {items && items.length > 0 && (
        <ul className="history">
          {items.map((h) => (
            <li key={h.id} className="history__item">
              <span className={`history__dot history__dot--${h.state}`} aria-hidden="true" />
              <div className="history__main">
                <p className="history__title" title={h.title}>
                  {h.title || h.url}
                </p>
                <p className="history__meta">
                  {h.state === "done" ? "Downloaded" : "Failed"} · {qualityLabel(h.quality)} · {formatDate(h.finishedAt)}
                </p>
                {h.state === "failed" && h.error && <p className="history__error">{h.error}</p>}
              </div>
              {h.state === "done" && h.filePath && (
                <button
                  type="button"
                  className="btn btn--ghost btn--sm"
                  onClick={() => api.openFolder(h.filePath ?? undefined).catch((e) => setErr(errorMessage(e)))}
                >
                  Show in folder
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={confirm}
        title="Clear history?"
        body="This removes the list of past downloads. Your video files stay where they are."
        confirmLabel="Clear history"
        danger
        busy={clearing}
        onConfirm={clear}
        onCancel={() => setConfirm(false)}
      />
    </section>
  );
}
