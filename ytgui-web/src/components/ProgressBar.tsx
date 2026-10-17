interface Props {
  /** 0-100, or null for an indeterminate (moving) bar. */
  percent: number | null;
  tone?: "active" | "done" | "error" | "idle";
  label: string;
  size?: "md" | "sm";
}

export function ProgressBar({ percent, tone = "active", label, size = "md" }: Props) {
  const clamped = percent == null ? null : Math.max(0, Math.min(100, percent));
  return (
    <div
      className={`progress progress--${tone} progress--${size}${clamped == null ? " progress--indeterminate" : ""}`}
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={clamped == null ? undefined : Math.round(clamped)}
    >
      <div className="progress__fill" style={clamped == null ? undefined : { width: `${clamped}%` }} />
    </div>
  );
}
