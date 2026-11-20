import { formatCount, formatMs, formatPercent, formatRate, relativeTime, shortId, timecode } from "./format";

describe("format", () => {
  it("prints timecodes with milliseconds", () => {
    const d = new Date(2026, 10, 21, 9, 5, 7, 42);
    expect(timecode(d)).toBe("09:05:07.042");
    expect(timecode(d, false)).toBe("09:05:07");
  });

  it("formats counts, rates, percentages and durations", () => {
    expect(formatCount(1234567)).toBe("1,234,567");
    expect(formatRate(0)).toBe("0/s");
    expect(formatRate(0.42)).toBe("0.4/s");
    expect(formatRate(97.4)).toBe("97/s");
    expect(formatPercent(0.9987)).toBe("99.9%");
    expect(formatPercent(1)).toBe("100%");
    expect(formatPercent(null)).toBe("–");
    expect(formatMs(84.4)).toBe("84 ms");
    expect(formatMs(1240)).toBe("1.24 s");
  });

  it("describes times relative to now", () => {
    const now = Date.UTC(2026, 10, 21, 12, 0, 0);
    expect(relativeTime(now + 400, now)).toBe("now");
    expect(relativeTime(now + 12_000, now)).toBe("in 12s");
    expect(relativeTime(now - 3 * 60_000, now)).toBe("3m ago");
    expect(relativeTime(now - 5 * 3_600_000, now)).toBe("5h ago");
  });

  it("shortens IDs to their first eight characters", () => {
    expect(shortId("01a1fb2c-4259-7f1c-95e0-3369fd6a70a8")).toBe("01a1fb2c");
  });
});
