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
});
