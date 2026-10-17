import { useRef } from "react";
import { ArrowRight, Clipboard, DownloadIcon, LinkIcon } from "./Icons";

export const EXAMPLE_URL = "https://www.youtube.com/watch?v=jNQXAC9IVRw";

interface Props {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  loading: boolean;
}

export function UrlInput({ value, onChange, onSubmit, loading }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);

  const paste = async () => {
    try {
      const text = await navigator.clipboard.readText();
      if (text.trim()) onChange(text.trim());
    } catch {
      /* clipboard blocked or empty: ignore */
    }
    inputRef.current?.focus();
  };

  return (
    <form
      className="urlbox"
      onSubmit={(e) => {
        e.preventDefault();
        if (!loading && value.trim()) onSubmit();
      }}
    >
      <div className="urlbox__head">
        <span className="iconsq" aria-hidden="true">
          <LinkIcon size={20} />
        </span>
        <div className="urlbox__heading">
          <label htmlFor="url" className="urlbox__label">
            Paste a YouTube link
          </label>
          <p id="url-hint" className="urlbox__sub">
            A video or Shorts link
          </p>
        </div>
        <button
          type="button"
          className="btn btn--outline btn--sm urlbox__example"
          disabled={loading}
          onClick={() => {
            onChange(EXAMPLE_URL);
            inputRef.current?.focus();
          }}
        >
          Try example
          <ArrowRight size={14} />
        </button>
      </div>

      <div className="urlbox__field">
        <LinkIcon size={18} className="urlbox__fieldicon" />
        <input
          ref={inputRef}
          id="url"
          className="urlbox__input"
          type="text"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          placeholder="https://www.youtube.com/watch?v=…"
          aria-describedby="url-hint"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={loading}
        />
        <button type="button" className="btn btn--soft btn--sm urlbox__paste" onClick={paste} disabled={loading}>
          <Clipboard size={16} />
          Paste
        </button>
      </div>

      <button type="submit" className="btn btn--primary btn--xl btn--block" disabled={loading || !value.trim()}>
        <DownloadIcon size={20} />
        {loading ? "Looking up the video…" : "Get download options"}
        {!loading && <ArrowRight size={18} className="btn__trail" />}
      </button>

      <p className="urlbox__foot">
        <span>No sign-up</span>
        <span>No ads</span>
        <span>Runs on your computer</span>
        <span>Free</span>
      </p>
    </form>
  );
}
