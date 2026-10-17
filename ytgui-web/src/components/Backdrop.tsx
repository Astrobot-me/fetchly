/** Page atmosphere: soft red glows and thin curved light lines. Purely decorative. */
export function Backdrop() {
  return (
    <div className="backdrop" aria-hidden="true">
      <div className="backdrop__glow backdrop__glow--a" />
      <div className="backdrop__glow backdrop__glow--b" />
      <div className="backdrop__glow backdrop__glow--c" />
      <svg className="backdrop__lines" viewBox="0 0 1600 1000" preserveAspectRatio="xMidYMid slice">
        <defs>
          <linearGradient id="bd-fade" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0" stopColor="var(--line-glow)" stopOpacity="0" />
            <stop offset="0.5" stopColor="var(--line-glow)" stopOpacity="1" />
            <stop offset="1" stopColor="var(--line-glow)" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d="M-80 120 C 260 40, 420 520, 820 640 S 1500 560, 1700 980" stroke="url(#bd-fade)" />
        <path d="M-120 620 C 220 520, 360 940, 760 1060" stroke="url(#bd-fade)" />
        <path d="M1700 80 C 1380 160, 1300 520, 980 700" stroke="url(#bd-fade)" />
        <path d="M280 -60 C 420 160, 360 360, 120 520" stroke="url(#bd-fade)" className="backdrop__thin" />
        <path d="M1320 -40 C 1240 220, 1420 380, 1620 420" stroke="url(#bd-fade)" className="backdrop__thin" />
      </svg>
    </div>
  );
}
