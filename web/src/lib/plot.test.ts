import type { Metrics } from "../api/client";
import { toBuckets } from "./plot";

describe("toBuckets", () => {
  it("turns counts into rates and drops the bucket still filling up", () => {
    const m = {
      generated_at: "2026-11-21T12:00:12Z",
      window_seconds: 300,
      bucket_seconds: 5,
      channels: [],
      status_counts: {},
      workers: [],
      throughput: [
        { at: "2026-11-21T12:00:00Z", channel: "sms", succeeded: 500, failed: 3 },
        { at: "2026-11-21T12:00:00Z", channel: "email", succeeded: 250, failed: 0 },
        { at: "2026-11-21T12:00:05Z", channel: "sms", succeeded: 480, failed: 1 },
        { at: "2026-11-21T12:00:10Z", channel: "sms", succeeded: 120, failed: 0 },
      ],
    } satisfies Metrics;

    const buckets = toBuckets(m);

    expect(buckets).toHaveLength(2);
    expect(buckets[0]?.rates).toEqual({ sms: 100, email: 50, push: 0 });
    expect(buckets[0]?.failed).toBe(3);
    expect(buckets[1]?.rates.sms).toBe(96);
  });

  it("holds back a bucket that closed moments before the snapshot", () => {
    const m = {
      generated_at: "2026-11-21T12:00:02.200Z",
      window_seconds: 60,
      bucket_seconds: 1,
      channels: [],
      status_counts: {},
      workers: [],
      throughput: [
        { at: "2026-11-21T12:00:00Z", channel: "sms", succeeded: 100, failed: 0 },
        // Closed 200 ms before the snapshot: sends from its last moments may
        // not be written yet, so it is not trusted.
        { at: "2026-11-21T12:00:01Z", channel: "sms", succeeded: 61, failed: 0 },
        { at: "2026-11-21T12:00:02Z", channel: "sms", succeeded: 18, failed: 0 },
      ],
    } satisfies Metrics;

    expect(toBuckets(m).map((b) => b.rates.sms)).toEqual([100]);
  });
});
