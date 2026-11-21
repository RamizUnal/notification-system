import type { ChannelMetrics, Worker } from "../api/client";
import type { LiveRate } from "../api/events";
import { formatCount, formatRate, relativeTime } from "./format";

export type Tally = "on-air" | "cued" | "idle" | "probing" | "open" | "no-worker";

export interface TallyView {
  state: Tally;
  label: string;
  detail: string;
}

/** tallyFor decides what the monitor's tally says, most severe state first. */
export function tallyFor(m: ChannelMetrics, live: LiveRate, workers: Worker[]): TallyView {
  if (m.circuit === "open") {
    const retryAt = workers
      .map((w) => w.circuits[m.channel]?.retry_at)
      .filter((t): t is string => Boolean(t))
      .sort()[0];
    return {
      state: "open",
      label: "Circuit open",
      // Short enough that the countdown, the part that changes, is never cut off.
      detail:
        retryAt && new Date(retryAt).getTime() > Date.now() ? `Next probe ${relativeTime(retryAt)}` : "Probe due",
    };
  }
  if (m.circuit === "half_open") return { state: "probing", label: "Probing", detail: "One test send to the provider" };
  if (m.circuit === "unknown") return { state: "no-worker", label: "No worker", detail: "No worker is reporting in" };
  if (live.rate > 0 || m.queue.processing > 0) return { state: "on-air", label: "On air", detail: `${formatRate(live.rate)} now` };
  if (m.queue.due > 0) return { state: "cued", label: "Cued", detail: `${formatCount(m.queue.due)} due` };
  return { state: "idle", label: "Idle", detail: "Nothing to send" };
}

