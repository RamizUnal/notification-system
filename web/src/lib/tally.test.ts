import type { ChannelMetrics, Worker } from "../api/client";
import { tallyFor } from "./tally";

function metrics(overrides: Partial<ChannelMetrics> = {}): ChannelMetrics {
  return {
    channel: "sms",
    queue: { due: 0, scheduled: 0, retrying: 0, processing: 0, by_priority: { high: 0, normal: 0, low: 0 } },
    deliveries: {
      succeeded: 0, retryable_failures: 0, permanent_failures: 0, success_rate: null,
      throughput_per_second: 0, latency_ms: { p50: 0, p95: 0, p99: 0 },
    },
    rate_limit_per_second: 100,
    circuit: "closed",
    ...overrides,
  };
}

const idle = { rate: 0, peak: 0 };

describe("tallyFor", () => {
  it("puts a broken circuit above everything else", () => {
    const workers: Worker[] = [{
      id: "w1", started_at: "", seen_at: "", in_flight: 0,
      circuits: { sms: { state: "open", retry_at: new Date(Date.now() + 12_000).toISOString() } },
    }];
    const t = tallyFor(metrics({ circuit: "open" }), { rate: 50, peak: 90 }, workers);
    expect(t.state).toBe("open");
    expect(t.label).toBe("Circuit open");
    expect(t.detail).toMatch(/^Next probe in 1[12]s$/);
  });

  it("says a probe is due once its time has passed", () => {
    expect(tallyFor(metrics({ circuit: "open" }), idle, []).detail).toBe("Probe due");
  });

  it("flags a channel without workers", () => {
    expect(tallyFor(metrics({ circuit: "unknown" }), idle, []).state).toBe("no-worker");
  });

  it("is on air while deliveries flow, cued while work waits, idle otherwise", () => {
    expect(tallyFor(metrics(), { rate: 97, peak: 100 }, []).state).toBe("on-air");
    expect(tallyFor(metrics({ queue: { ...metrics().queue, due: 12 } }), idle, []).state).toBe("cued");
    expect(tallyFor(metrics(), idle, []).state).toBe("idle");
  });
});
