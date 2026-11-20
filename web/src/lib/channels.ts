import { Mail, MessageSquareText, Smartphone, type LucideIcon } from "lucide-react";

import type { Channel, Priority } from "../api/client";

export const CHANNELS: readonly Channel[] = ["sms", "email", "push"];
export const PRIORITIES: readonly Priority[] = ["high", "normal", "low"];

interface ChannelInfo {
  label: string;
  recipientLabel: string;
  placeholder: string;
  hint: string;
  /** Mirrors the server's limit for live counting; the server stays authoritative. */
  maxLength: number;
  Icon: LucideIcon;
}

export const channelInfo: Record<Channel, ChannelInfo> = {
  sms: {
    label: "SMS",
    recipientLabel: "Phone number",
    placeholder: "+905551234567",
    hint: "E.164: a plus sign, the country code, then the number.",
    maxLength: 1600,
    Icon: MessageSquareText,
  },
  email: {
    label: "Email",
    recipientLabel: "Email address",
    placeholder: "ada@example.com",
    hint: "A plain address without a display name.",
    maxLength: 10_000,
    Icon: Mail,
  },
  push: {
    label: "Push",
    recipientLabel: "Device token",
    placeholder: "fcm:APA91bH-device-token",
    hint: "16 to 512 characters: letters, digits and . _ : -",
    maxLength: 500,
    Icon: Smartphone,
  },
};

export const priorityLabel: Record<Priority, string> = { high: "High", normal: "Normal", low: "Low" };
