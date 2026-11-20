import { smsSegments } from "./sms";

describe("smsSegments", () => {
  it("fits 160 GSM-7 characters in one segment", () => {
    expect(smsSegments("a".repeat(160))).toEqual({ encoding: "GSM-7", segments: 1, remaining: 0 });
  });

  it("splits longer GSM-7 messages into 153-character segments", () => {
    expect(smsSegments("a".repeat(161))).toEqual({ encoding: "GSM-7", segments: 2, remaining: 145 });
  });

  it("counts extension characters twice", () => {
    expect(smsSegments("€".repeat(80)).segments).toBe(1);
    expect(smsSegments("€".repeat(81)).segments).toBe(2);
  });

  it("switches to UCS-2 for characters outside GSM-7, such as Turkish ş", () => {
    const turkish = "Siparişiniz yolda";
    expect(smsSegments(turkish)).toEqual({ encoding: "UCS-2", segments: 1, remaining: 70 - turkish.length });
    expect(smsSegments("ş".repeat(71)).segments).toBe(2);
  });

  it("reports an empty message as zero segments", () => {
    expect(smsSegments("").segments).toBe(0);
  });
});
