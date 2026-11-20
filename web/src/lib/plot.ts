import type { Channel, Metrics } from "../api/client";

export interface Bucket {
  at: number;
  rates: Record<Channel, number>;
  failed: number;
}

/** Turns the metrics series into per-bucket rates, dropping the bucket still filling up. */
export function toBuckets(metrics: Metrics): Bucket[] {
  const bucketMs = metrics.bucket_seconds * 1000;
  const generated = new Date(metrics.generated_at).getTime();
  const byTime = new Map<number, Bucket>();
  for (const p of metrics.throughput) {
    const at = new Date(p.at).getTime();
    if (at + bucketMs > generated) continue;
    const bucket = byTime.get(at) ?? { at, rates: { sms: 0, email: 0, push: 0 }, failed: 0 };
    bucket.rates[p.channel] = p.succeeded / metrics.bucket_seconds;
    bucket.failed += p.failed;
    byTime.set(at, bucket);
  }
  return [...byTime.values()].sort((a, b) => a.at - b.at);
}

