import { useEffect, useRef, useState } from "react";
import { api, errorMessage } from "../api/client";
import type { VideoInfo } from "../api/types";
import { FormatPicker } from "../components/FormatPicker";
import { JobList } from "../components/JobList";
import { UrlInput } from "../components/UrlInput";
import { formatDuration, pickDefaultQuality } from "../format";
import { actions, useStore } from "../state/store";

export function Home() {
  const { settings } = useStore();
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [slow, setSlow] = useState(false);
  const [findError, setFindError] = useState<string | null>(null);
  const [info, setInfo] = useState<VideoInfo | null>(null);
  const [quality, setQuality] = useState("best");
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);
  const [added, setAdded] = useState<string | null>(null);
  const requestId = useRef(0);

  // Show a gentler message if the lookup takes a while.
  useEffect(() => {
    if (!loading) {
      setSlow(false);
      return;
    }
    const t = window.setTimeout(() => setSlow(true), 8000);
    return () => window.clearTimeout(t);
  }, [loading]);

  const find = async () => {
    const id = ++requestId.current;
    setLoading(true);
    setFindError(null);
    setInfo(null);
    setAdded(null);
    setAddError(null);
    try {
      const v = await api.info(url.trim());
      if (id !== requestId.current) return;
      setInfo(v);
      setQuality(pickDefaultQuality(v.qualities, settings?.defaultQuality));
    } catch (e) {
      if (id !== requestId.current) return;
      setFindError(errorMessage(e));
    } finally {
      if (id === requestId.current) setLoading(false);
    }
  };

  const download = async () => {
    if (!info) return;
    setAdding(true);
    setAddError(null);
    try {
      const job = await api.createJob({
        url: info.url,
        quality,
        title: info.title,
        thumbnail: info.thumbnail ?? undefined,
      });
      actions.upsertJob(job);
      setAdded(info.title);
      setInfo(null);
      setUrl("");
    } catch (e) {
      setAddError(errorMessage(e));
    } finally {
      setAdding(false);
    }
  };

  return (
    <div className="page page--home">
      <section className="hero">
        <h1 className="hero__title">
          What would you like to <em>fetch</em>?
        </h1>
        <UrlInput
          value={url}
          onChange={(v) => {
            setUrl(v);
            setFindError(null);
          }}
          onSubmit={find}
          loading={loading}
        />

        {loading && (
          <div className="lookup" role="status" aria-live="polite">
            <span className="spinner" aria-hidden="true" />
            <div>
              <p className="lookup__title">Looking up the video…</p>
              <p className="lookup__text">
                {slow ? "Still working. Some videos take up to 30 seconds." : "This can take a few seconds."}
              </p>
            </div>
          </div>
        )}

        {findError && (
          <p className="form-error" role="alert">
            {findError}
          </p>
        )}

        {added && !info && !loading && (
          <p className="form-ok" role="status">
            Added “{added}” to your downloads.
          </p>
        )}

        {info && (
          <article className="video" aria-label="Video found">
            <div className="video__thumb">
              {info.thumbnail ? (
                <img src={info.thumbnail} alt="" referrerPolicy="no-referrer" />
              ) : (
                <span className="job__thumb-blank" aria-hidden="true" />
              )}
              {info.durationSec != null && (
                <span className="video__duration">{formatDuration(info.durationSec)}</span>
              )}
            </div>
            <div className="video__body">
              <h2 className="video__title">{info.title}</h2>
              {info.uploader && <p className="video__by">{info.uploader}</p>}

              <FormatPicker options={info.qualities} value={quality} onChange={setQuality} disabled={adding} />

              {addError && (
                <p className="form-error" role="alert">
                  {addError}
                </p>
              )}

              <div className="video__actions">
                <button type="button" className="btn btn--primary btn--xl" onClick={download} disabled={adding}>
                  <svg aria-hidden="true" viewBox="0 0 24 24" width="20" height="20">
                    <path
                      d="M12 4v11m-5-4.5 5 5 5-5M5 20h14"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2.2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    />
                  </svg>
                  {adding ? "Adding…" : "Download"}
                </button>
                <button
                  type="button"
                  className="btn btn--ghost"
                  onClick={() => {
                    setInfo(null);
                    setAddError(null);
                  }}
                  disabled={adding}
                >
                  Not this one
                </button>
              </div>
            </div>
          </article>
        )}
      </section>

      <JobList />
    </div>
  );
}
