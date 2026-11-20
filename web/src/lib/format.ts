const clock = new Intl.DateTimeFormat("en-GB", {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});
const dateTime = new Intl.DateTimeFormat("en-GB", {
  day: "numeric",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});
const count = new Intl.NumberFormat("en-US");

function toDate(value: string | number | Date): Date {
  return value instanceof Date ? value : new Date(value);
}

/** Broadcast-style timecode in local time: 22:12:05.521 */
export function timecode(value: string | number | Date, withMillis = true): string {
  const d = toDate(value);
  const base = clock.format(d);
  return withMillis ? `${base}.${String(d.getMilliseconds()).padStart(3, "0")}` : base;
}

/** 21 Nov, 00:52:04 */
export function formatDateTime(value: string | number | Date): string {
  return dateTime.format(toDate(value));
}

export function formatCount(n: number): string {
  return count.format(Math.round(n));
}

/** 97/s, 0.4/s */
export function formatRate(perSecond: number): string {
  if (perSecond === 0) return "0/s";
  return perSecond >= 10 ? `${Math.round(perSecond)}/s` : `${perSecond.toFixed(1)}/s`;
}

/** 99.9%, or an en dash when there is nothing to measure. */
export function formatPercent(ratio: number | null | undefined, digits = 1): string {
  if (ratio === null || ratio === undefined || Number.isNaN(ratio)) return "–";
  const pct = ratio * 100;
  return pct === 100 ? "100%" : `${pct.toFixed(digits)}%`;
}

/** 84 ms, 1.24 s */
export function formatMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}

/**
 * The last eight characters of an ID. IDs are UUIDv7, whose leading
 * characters encode the creation time, so everything created within a
 * minute shares a prefix; the random tail is what tells two IDs apart.
 */
export function shortId(id: string): string {
  return id.slice(-8);
}

/** in 2s, 3s ago, 2m ago, 4h ago */
export function relativeTime(value: string | number | Date, now = Date.now()): string {
  const diff = toDate(value).getTime() - now;
  const abs = Math.abs(diff);
  let text: string;
  if (abs < 1000) return "now";
  if (abs < 60_000) text = `${Math.round(abs / 1000)}s`;
  else if (abs < 3_600_000) text = `${Math.round(abs / 60_000)}m`;
  else if (abs < 86_400_000) text = `${Math.round(abs / 3_600_000)}h`;
  else text = `${Math.round(abs / 86_400_000)}d`;
  return diff > 0 ? `in ${text}` : `${text} ago`;
}
