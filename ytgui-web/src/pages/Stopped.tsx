export function Stopped() {
  return (
    <div className="screen">
      <div className="screen__card">
        <span className="screen__icon screen__icon--calm" aria-hidden="true">
          <svg viewBox="0 0 24 24" width="28" height="28">
            <path d="M9 8v8M15 8v8" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" />
          </svg>
        </span>
        <h1 className="screen__title">The app has stopped.</h1>
        <p className="screen__text">You can close this tab.</p>
        <p className="screen__hint">To download more, open Fetchly again from your computer.</p>
      </div>
    </div>
  );
}
