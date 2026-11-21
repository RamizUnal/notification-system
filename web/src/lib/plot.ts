import type { Channel, Metrics } from "../api/client";

/**
 * How long a bucket must have been closed before it counts as settled.
 * Attempts that finish just before a boundary are still being written when
 * the boundary passes, so a bucket that closed moments ago reads low.
 */
export const SETTLE_MS = 500;

export interface Bucket {
  at: number;
  rates: Record<Channel, number>;
  failed: number;
}

/**
 * Turns the metrics series into per-bucket rates, keeping only settled
 * buckets: neither the one still filling up nor one that has only just closed.
 */
export function toBuckets(metrics: Metrics): Bucket[] {
  const bucketMs = metrics.bucket_seconds * 1000;
  const settledBy = new Date(metrics.generated_at).getTime() - SETTLE_MS;
  const byTime = new Map<number, Bucket>();
  for (const p of metrics.throughput) {
    const at = new Date(p.at).getTime();
    if (at + bucketMs > settledBy) continue;
    const bucket = byTime.get(at) ?? { at, rates: { sms: 0, email: 0, push: 0 }, failed: 0 };
    bucket.rates[p.channel] = p.succeeded / metrics.bucket_seconds;
    bucket.failed += p.failed;
    byTime.set(at, bucket);
  }
  return [...byTime.values()].sort((a, b) => a.at - b.at);
}
