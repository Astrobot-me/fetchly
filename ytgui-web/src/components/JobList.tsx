import { useState } from "react";
import { api, errorMessage } from "../api/client";
import type { Job, ToolProgress } from "../api/types";
import { fileName, formatEta, formatSpeed, qualityLabel } from "../format";
import { actions, useStore } from "../state/store";
import { Folder, Retry } from "./Icons";
import { ProgressBar } from "./ProgressBar";

const FINISHED: Job["state"][] = ["done", "failed", "canceled"];

export function JobList() {
  const { jobs, queuePaused, tools, toolProgress } = useStore();
  const [resuming, setResuming] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const hasFinished = jobs.some((j) => FINISHED.includes(j.state));
  const ytdlp = tools?.tools.find((t) => t.name === "yt-dlp");

  const resume = async () => {
    setResuming(true);
    setErr(null);
    try {
      const r = await api.resumeQueue();
      actions.setJobs(r.jobs, r.queuePaused);
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setResuming(false);
    }
  };

  const clear = async () => {
    setClearing(true);
    setErr(null);
    try {
      const r = await api.clearJobs();
      actions.setJobs(r.jobs, r.queuePaused);
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setClearing(false);
    }
  };

  return (
    <section className="jobs" aria-labelledby="jobs-title">
      <div className="jobs__head">
        <h2 id="jobs-title" className="section-title">
          Your downloads
          {jobs.length > 0 && <span className="count">{jobs.length}</span>}
        </h2>
        {hasFinished && (
          <button type="button" className="btn btn--ghost btn--sm" onClick={clear} disabled={clearing}>
            Clear finished
          </button>
        )}
      </div>

      {queuePaused && (
        <div className="banner banner--warn" role="status">
          <span className="banner__dot" aria-hidden="true" />
          <p className="banner__text">Downloads paused because one failed.</p>
          <button type="button" className="btn btn--primary btn--sm" onClick={resume} disabled={resuming}>
            Resume
          </button>
        </div>
      )}
      {err && <p className="form-error">{err}</p>}

      {jobs.length === 0 ? (
        <div className="empty">
          <p className="empty__title">No downloads yet</p>
          <p className="empty__text">Paste a link above. Videos show up here while they're saving.</p>
        </div>
      ) : (
        <ul className="joblist">
          {jobs.map((j) => (
            <JobRow
              key={j.id}
              job={j}
              updaterBusy={!!ytdlp?.busy}
              updaterProgress={toolProgress["yt-dlp"]}
            />
          ))}
        </ul>
      )}
    </section>
  );
}

interface RowProps {
  job: Job;
  updaterBusy: boolean;
  updaterProgress?: ToolProgress;
}

function JobRow({ job, updaterBusy, updaterProgress }: RowProps) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [updateRequested, setUpdateRequested] = useState(false);

  const act = async (fn: () => Promise<unknown>) => {
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

  const active = job.state === "running" || job.state === "queued";
  const finished = !active;

  let phase = job.phase;
  if (job.state === "queued") phase = job.phase || "Waiting";
  if (job.state === "done") phase = "Finished";
  if (job.state === "canceled") phase = "Canceled";
  if (job.state === "failed") phase = "Didn't finish";

  const tone =
    job.state === "done" ? "done" : job.state === "failed" ? "error" : job.state === "running" ? "active" : "idle";
  const percent = job.state === "done" ? 100 : job.state === "queued" ? 0 : job.percent;

  const speed = job.state === "running" ? formatSpeed(job.speed) : "";
  const eta = job.state === "running" ? formatEta(job.eta) : "";

  return (
    <li className={`job job--${job.state}`}>
      <div className="job__thumb">
        {job.thumbnail ? (
          <img src={job.thumbnail} alt="" referrerPolicy="no-referrer" loading="lazy" />
        ) : (
          <span className="job__thumb-blank" aria-hidden="true" />
        )}
      </div>

      <div className="job__main">
        <p className="job__title" title={job.title}>
          {job.title || job.url}
        </p>
        <p className="job__meta">
          <span className={`pill pill--${job.state}`}>{phase}</span>
          <span className="job__quality">{qualityLabel(job.quality)}</span>
          {job.attempt > 1 && active && <span className="job__retry">Retrying ({job.attempt} of 3)</span>}
        </p>

        {(active || job.state === "done") && (
          <ProgressBar
            percent={job.state === "running" && job.percent <= 0 && !job.speed ? null : percent}
            tone={tone}
            label={`${job.title}: ${phase}`}
          />
        )}

        {(speed || eta) && (
          <p className="job__numbers">
            <span>{Math.round(job.percent)}%</span>
            {speed && <span>{speed}</span>}
            {eta && <span>{eta}</span>}
          </p>
        )}

        {job.state === "done" && job.filePath && (
          <p className="job__file" title={job.filePath}>
            Saved as {fileName(job.filePath)}
          </p>
        )}

        {job.state === "failed" && job.error && <p className="job__error">{job.error}</p>}

        {job.state === "failed" && job.suggestUpdate && (
          <div className="job__hint">
            <p>This often means the downloader needs an update.</p>
            {updaterBusy ? (
              <div className="job__hint-progress">
                <span>{updaterProgress?.message || "Updating…"}</span>
                <ProgressBar
                  percent={updaterProgress && updaterProgress.percent > 0 ? updaterProgress.percent : null}
                  label="Updating the downloader"
                  size="sm"
                />
              </div>
            ) : updateRequested && updaterProgress?.stage === "done" ? (
              <p className="job__hint-done">Update finished. Press Retry to try again.</p>
            ) : updateRequested && updaterProgress?.stage !== "error" ? (
              <p className="job__hint-done">Starting the update…</p>
            ) : (
              <button
                type="button"
                className="btn btn--primary btn--sm"
                disabled={busy}
                onClick={() =>
                  act(async () => {
                    await api.updateTool("yt-dlp");
                    setUpdateRequested(true);
                  })
                }
              >
                Update
              </button>
            )}
          </div>
        )}

        {err && <p className="job__error">{err}</p>}
      </div>

      <div className="job__actions">
        {job.state === "done" && job.filePath && (
          <button
            type="button"
            className="btn btn--soft btn--sm"
            disabled={busy}
            onClick={() => act(() => api.openFolder(job.filePath ?? undefined))}
          >
            <Folder size={16} />
            Show in folder
          </button>
        )}
        {(job.state === "failed" || job.state === "canceled") && (
          <button
            type="button"
            className="btn btn--primary btn--sm"
            disabled={busy}
            onClick={() => act(async () => actions.upsertJob(await api.retryJob(job.id)))}
          >
            <Retry size={16} />
            Retry
          </button>
        )}
        {active && (
          <button
            type="button"
            className="btn btn--ghost btn--sm"
            disabled={busy}
            onClick={() => act(() => api.deleteJob(job.id))}
          >
            Cancel
          </button>
        )}
        {finished && (
          <button
            type="button"
            className="btn btn--ghost btn--sm"
            disabled={busy}
            onClick={() =>
              act(async () => {
                await api.deleteJob(job.id);
                actions.removeJob(job.id);
              })
            }
          >
            Remove
          </button>
        )}
      </div>
    </li>
  );
}
