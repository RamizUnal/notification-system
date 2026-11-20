import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ApiError, type Channel } from "./client";

// The mock provider's admin API (proxied at /mock). It exists only when the
// stack delivers to the bundled mock; with webhook.site configured the
// console says so and hides the simulator controls.

export interface ProviderSettings {
  failure_rate: number;
  min_latency_ms: number;
  max_latency_ms: number;
  outage: boolean;
}

export interface ProviderMessage {
  message_id?: string;
  to: string;
  channel: Channel;
  content: string;
  status: number;
  idempotency_key?: string;
  correlation_id?: string;
  received_at: string;
}

export interface ProviderStats {
  settings: ProviderSettings;
  totals: Record<"received" | "accepted" | "failed" | "rejected", number>;
  channels: Record<Channel, { last_second: number; peak_per_second: number }>;
  recent: ProviderMessage[];
}

export type ProviderMode = "stable" | "flaky" | "outage";

export const modeSettings: Record<ProviderMode, Partial<ProviderSettings>> = {
  stable: { outage: false, failure_rate: 0.03 },
  flaky: { outage: false, failure_rate: 0.35 },
  outage: { outage: true },
};

export function modeOf(settings: ProviderSettings): ProviderMode {
  if (settings.outage) return "outage";
  return settings.failure_rate >= 0.2 ? "flaky" : "stable";
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`/mock${path}`, init);
  } catch {
    throw new ApiError({ title: "Unreachable", status: 0, code: "network_error", detail: "The provider simulator is not reachable." });
  }
  if (!res.ok) {
    throw new ApiError({
      title: res.statusText,
      status: res.status,
      code: "provider_simulator_error",
      detail: res.status === 404 || res.status === 502
        ? "The provider simulator is not running; the stack is probably delivering to an external provider."
        : `The provider simulator answered ${res.status}.`,
    });
  }
  return (await res.json()) as T;
}

export function useProviderStats(interval = 1000) {
  return useQuery({
    queryKey: ["provider", "stats"],
    queryFn: () => request<ProviderStats>("/admin/stats"),
    refetchInterval: interval,
    retry: false,
  });
}

export function useUpdateProvider() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (settings: ProviderSettings) =>
      request<ProviderSettings>("/admin/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(settings),
      }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["provider"] }),
  });
}
