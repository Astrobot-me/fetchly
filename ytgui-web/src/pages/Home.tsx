import { type ReactNode, useEffect, useRef, useState } from "react";
import { api, errorMessage } from "../api/client";
import type { VideoInfo } from "../api/types";
import { FormatPicker } from "../components/FormatPicker";
import { JobList } from "../components/JobList";
import { Code, DownloadIcon, Film, Layers, Lock, Music, Phone, Screen } from "../components/Icons";
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
      <header className="hero">
        <h1 className="hero__word">
          Fetch<span className="hero__ly">ly</span>
        </h1>
        <p className="hero__tag">
          Grab YouTube videos. <span className="hero__accent">Instantly.</span>
        </p>
        <p className="hero__sub">
          Paste a link, pick a quality, and it's saved to your computer. No sign-up, no ads.
        </p>
      </header>

      <div className="stage">
        <ul className="tiles tiles--side" aria-label="What Fetchly does">
          {TILES.slice(0, 3).map(renderTile)}
        </ul>

        <section className="fetchcard" aria-label="Download a video">
          <span className="fetchcard__orbit" aria-hidden="true" />
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
              <div className="video__top">
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
                <div className="video__text">
                  <h2 className="video__title">{info.title}</h2>
                  {info.uploader && <p className="video__by">{info.uploader}</p>}
                </div>
              </div>

              <FormatPicker options={info.qualities} value={quality} onChange={setQuality} disabled={adding} />

              {addError && (
                <p className="form-error" role="alert">
                  {addError}
                </p>
              )}

              <div className="video__actions">
                <button type="button" className="btn btn--primary btn--xl" onClick={download} disabled={adding}>
                  <DownloadIcon size={20} />
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
            </article>
          )}
        </section>

        <ul className="tiles tiles--side" aria-label="How it works">
          {TILES.slice(3).map(renderTile)}
        </ul>
      </div>

      <JobList />

      <ul className="tiles tiles--strip" aria-label="What Fetchly does">
        {TILES.map(renderTile)}
      </ul>

      <section className="supports" aria-labelledby="supports-title">
        <h2 id="supports-title" className="supports__title">
          Fetchly supports…
        </h2>
        <ul className="supports__list">
          <Support icon={<Film />} title="Videos" text="Normal YouTube links" />
          <Support icon={<Phone />} title="Shorts" text="youtube.com/shorts links" />
          <Support icon={<Music />} title="Audio" text="M4A, as uploaded" />
          <Support icon={<Screen />} title="Qualities" text="144p to 4K, 8K if offered" />
          <Support icon={<DownloadIcon />} title="MP4" text="H.264 up to 1080p" />
        </ul>
        <p className="supports__note">Playlists aren't supported yet. Only download videos you have the right to download.</p>
      </section>
    </div>
  );
}

const TILES: { icon: ReactNode; title: string; text: string }[] = [
  { icon: <Screen />, title: "Up to 4K", text: "Pick 144p to 4K, or 8K when the video has it." },
  { icon: <Film />, title: "Plays anywhere", text: "Saves MP4 that opens in any player." },
  { icon: <Music />, title: "Audio only", text: "Keep just the sound as an M4A file." },
  { icon: <Lock />, title: "On your computer", text: "No account and no server in between." },
  { icon: <Layers />, title: "Queue and retry", text: "Line up several videos. Failed ones retry up to twice." },
  { icon: <Code />, title: "Free and open source", text: "Its tools update only when you click Update." },
];

function renderTile(t: (typeof TILES)[number]) {
  return (
    <li key={t.title} className="tile">
      <span className="iconsq" aria-hidden="true">
        {t.icon}
      </span>
      <div>
        <p className="tile__title">{t.title}</p>
        <p className="tile__text">{t.text}</p>
      </div>
    </li>
  );
}

function Support({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return (
    <li className="support">
      <span className="iconsq iconsq--sm" aria-hidden="true">
        {icon}
      </span>
      <span>
        <span className="support__title">{title}</span>
        <span className="support__text">{text}</span>
      </span>
    </li>
  );
}
