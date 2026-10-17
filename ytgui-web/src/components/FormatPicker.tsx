import type { QualityOption } from "../api/types";
import { formatBytes } from "../format";

interface Props {
  options: QualityOption[];
  value: string;
  onChange: (id: string) => void;
  disabled?: boolean;
}

export function FormatPicker({ options, value, onChange, disabled }: Props) {
  return (
    <fieldset className="picker" disabled={disabled}>
      <legend className="picker__legend">Choose quality</legend>
      <div className="picker__grid">
        {options.map((o) => {
          const size = formatBytes(o.approxBytes);
          const checked = o.id === value;
          return (
            <label key={o.id} className={`chip${checked ? " chip--on" : ""}${o.id === "audio" ? " chip--audio" : ""}`}>
              <input
                type="radio"
                name="quality"
                value={o.id}
                checked={checked}
                onChange={() => onChange(o.id)}
              />
              <span className="chip__label">{o.label}</span>
              <span className="chip__size">{size ? `about ${size}` : "size unknown"}</span>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}
