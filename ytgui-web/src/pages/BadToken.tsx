export function BadToken() {
  return (
    <div className="screen">
      <div className="screen__card">
        <span className="screen__icon screen__icon--warn" aria-hidden="true">
          <svg viewBox="0 0 24 24" width="28" height="28">
            <path d="M12 7v6m0 4h.01" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" />
          </svg>
        </span>
        <h1 className="screen__title">This page has lost its connection to Fetchly.</h1>
        <p className="screen__text">Please reopen the app from its launcher window.</p>
        <p className="screen__hint">You can close this tab afterwards.</p>
      </div>
    </div>
  );
}
