import { useRef } from "react";

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
      <label htmlFor="url" className="urlbox__label">
        Paste a YouTube link
      </label>
      <div className="urlbox__row">
        <div className="urlbox__field">
          <input
            ref={inputRef}
            id="url"
            className="urlbox__input"
            type="text"
            inputMode="url"
            autoComplete="off"
            spellCheck={false}
            placeholder="https://www.youtube.com/watch?v=…"
            value={value}
            onChange={(e) => onChange(e.target.value)}
            disabled={loading}
          />
          <button type="button" className="btn btn--soft urlbox__paste" onClick={paste} disabled={loading}>
            <svg aria-hidden="true" viewBox="0 0 24 24" width="18" height="18">
              <path
                d="M9 4h6m-6 0a2 2 0 0 0-2 2v0h10v0a2 2 0 0 0-2-2M9 4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1M7 6H6a2 2 0 0 0-2 2v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-1"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
              />
            </svg>
            Paste
          </button>
        </div>
        <button type="submit" className="btn btn--primary btn--xl" disabled={loading || !value.trim()}>
          {loading ? "Looking…" : "Find video"}
        </button>
      </div>
    </form>
  );
}
