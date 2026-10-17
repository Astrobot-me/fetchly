import type { QualityOption, ToolName } from "./api/types";

export function formatBytes(bytes: number | null | undefined): string {
  if (bytes == null || !isFinite(bytes) || bytes <= 0) return "";
  const units = ["bytes", "KB", "MB", "GB", "TB"];
  let v = bytes;
  let i = 0;
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000;
    i++;
  }
  const digits = v >= 100 || i === 0 ? 0 : v >= 10 ? 0 : 1;
  return `${v.toFixed(digits)} ${units[i]}`;
}

export function formatSpeed(bps: number | null): string {
  const b = formatBytes(bps);
  return b ? `${b}/s` : "";
}

export function formatEta(sec: number | null): string {
  if (sec == null || !isFinite(sec) || sec < 0) return "";
  if (sec < 5) return "almost done";
  if (sec < 60) return `about ${Math.round(sec)} sec left`;
  const min = Math.round(sec / 60);
  if (min < 60) return `about ${min} min left`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return `about ${h} h ${m} min left`;
}

export function formatDuration(sec: number | null): string {
  if (sec == null || !isFinite(sec)) return "";
  const s = Math.round(sec);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = s % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(r)}` : `${m}:${pad(r)}`;
}

export function formatDate(iso: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "";
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

/** Last path segment, for both Windows and POSIX paths. */
export function fileName(path: string | null): string {
  if (!path) return "";
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] || path;
}

export const QUALITY_CHOICES: { id: string; label: string }[] = [
  { id: "best", label: "Best available" },
  { id: "2160", label: "2160p (4K)" },
  { id: "1440", label: "1440p" },
  { id: "1080", label: "1080p (Full HD)" },
  { id: "720", label: "720p (HD)" },
  { id: "480", label: "480p" },
  { id: "360", label: "360p" },
  { id: "240", label: "240p" },
  { id: "144", label: "144p" },
  { id: "audio", label: "Audio only" },
];

export function qualityLabel(id: string): string {
  return QUALITY_CHOICES.find((q) => q.id === id)?.label ?? id;
}

/**
 * Default pick: the user's default quality if offered, else the nearest lower
 * resolution, else "best", else the first option.
 */
export function pickDefaultQuality(options: QualityOption[], preferred: string | undefined): string {
  if (options.length === 0) return "best";
  const has = (id: string) => options.some((o) => o.id === id);
  if (preferred && has(preferred)) return preferred;
  const wanted = preferred ? Number(preferred) : NaN;
  if (!isNaN(wanted)) {
    const lower = options
      .filter((o) => o.height != null && o.height <= wanted)
      .sort((a, b) => (b.height ?? 0) - (a.height ?? 0));
    if (lower.length > 0) return lower[0].id;
  }
  if (has("best")) return "best";
  return options[0].id;
}

/** Plain-language names for the helper programs. */
export const TOOL_INFO: Record<ToolName, { friendly: string; what: string }> = {
  "yt-dlp": { friendly: "Video downloader", what: "Fetches videos from YouTube" },
  ffmpeg: { friendly: "Video converter", what: "Joins picture and sound into one file" },
  deno: { friendly: "YouTube helper", what: "Recommended – improves YouTube support" },
};
