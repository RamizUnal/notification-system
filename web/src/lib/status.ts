import type { Status } from "../api/client";

/** Tally tones: colour only ever marks one of these states. */
export type Tone = "neutral" | "muted" | "live" | "probe" | "clear" | "alarm";

export const STATUSES: readonly Status[] = ["scheduled", "queued", "processing", "retrying", "sent", "failed", "cancelled"];

export const statusInfo: Record<Status, { label: string; tone: Tone; hint: string }> = {
  scheduled: { label: "Scheduled", tone: "muted", hint: "Waits for its delivery time" },
  queued: { label: "Queued", tone: "neutral", hint: "Waits for a worker" },
  processing: { label: "Processing", tone: "live", hint: "Being delivered right now" },
  retrying: { label: "Retrying", tone: "probe", hint: "Failed transiently; the next attempt is scheduled" },
  sent: { label: "Sent", tone: "clear", hint: "Accepted by the provider" },
  failed: { label: "Failed", tone: "alarm", hint: "Failed permanently or ran out of attempts" },
  cancelled: { label: "Cancelled", tone: "muted", hint: "Cancelled before delivery" },
};

export function isWaiting(status: Status): boolean {
  return status === "scheduled" || status === "queued" || status === "retrying";
}
