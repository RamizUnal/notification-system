import type { Channel, NotificationInput, Priority } from "../api/client";

// Synthetic traffic for demonstrations. Every message says it is a demo, so
// nothing on screen can be mistaken for real customer data.

const recipients: Record<Channel, string[]> = {
  sms: ["+905551234567", "+905321112233", "+447700900123", "+14155550142", "+4915112345678"],
  email: ["ada@example.com", "grace@example.com", "linus@example.org", "margaret@example.net"],
  push: ["fcm:APA91bH-demo-device-01", "fcm:APA91bH-demo-device-02", "apns-demo-0123456789abcdef"],
};

const messages: Record<Channel, string[]> = {
  sms: [
    "Flash sale: 40% off for the next 2 hours.",
    "Your verification code is 482910.",
    "Your order has shipped and arrives tomorrow.",
  ],
  email: [
    "Your weekly picks are ready. Open to see what is new this week.",
    "Your cart is waiting: the items you saved are almost sold out.",
    "Welcome aboard! Here is how to get started.",
  ],
  push: ["Breaking: the flash sale starts now.", "Back in stock: the sneakers you liked.", "Your driver is 2 minutes away."],
};

export interface BurstPreset {
  id: string;
  label: string;
  description: string;
  build: () => NotificationInput[];
}

export function synthesize(
  count: number,
  channels: readonly Channel[],
  priorityFor: (i: number) => Priority,
): NotificationInput[] {
  const runTag = Math.random().toString(36).slice(2, 6);
  return Array.from({ length: count }, (_, i) => {
    const channel = channels[i % channels.length] ?? "sms";
    const pick = <T,>(list: T[]) => list[i % list.length] as T;
    return {
      channel,
      priority: priorityFor(i),
      recipient: pick(recipients[channel]),
      content: `${pick(messages[channel])} (demo ${runTag}-${i + 1})`,
    };
  });
}

const cycle: Priority[] = ["high", "normal", "normal", "low"];

export const burstPresets: BurstPreset[] = [
  {
    id: "mixed",
    label: "1,000 mixed",
    description: "Every channel and priority; fills all three monitors",
    build: () => synthesize(1000, ["sms", "email", "push"], (i) => cycle[i % cycle.length] ?? "normal"),
  },
  {
    id: "sms-high",
    label: "500 SMS, high",
    description: "Drains at the 100/s ceiling in about five seconds",
    build: () => synthesize(500, ["sms"], () => "high"),
  },
  {
    id: "email-low",
    label: "300 email, low",
    description: "Low priority waits behind any high-priority work",
    build: () => synthesize(300, ["email"], () => "low"),
  },
  {
    id: "push-normal",
    label: "100 push",
    description: "A short push campaign",
    build: () => synthesize(100, ["push"], () => "normal"),
  },
];
