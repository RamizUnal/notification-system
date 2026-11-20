import { type QueryClient } from "@tanstack/react-query";

import type { Notification } from "./client";
import type { LiveEvent } from "./events";
import { keys, type NotificationPages } from "./queries";

const BATCH_REFRESH_MS = 500;

/**
 * applyEvents patches cached server state with streamed status changes, so
 * open tables and detail views update in place without refetching. Rows
 * never jump: new notifications are announced by the page, not inserted.
 */
export function createEventApplier(client: QueryClient): (events: LiveEvent[]) => void {
  const pendingBatches = new Set<string>();
  let batchTimer: ReturnType<typeof setTimeout> | undefined;

  return (events) => {
    const latest = new Map<string, LiveEvent>();
    for (const e of events) {
      if (e.batchId) pendingBatches.add(e.batchId);
      if (e.notificationId && e.status) latest.set(e.notificationId, e);
    }

    if (latest.size > 0) {
      const patch = (n: Notification): Notification => {
        const e = latest.get(n.id);
        if (!e?.status) return n;
        return { ...n, status: e.status, attempts: e.attempts, last_error: e.lastError ?? null, updated_at: e.occurredAt };
      };

      client.setQueriesData<NotificationPages>({ queryKey: keys.lists() }, (data) =>
        data
          ? { ...data, pages: data.pages.map((page) => ({ ...page, data: page.data.map(patch) })) }
          : data,
      );

      for (const [id, e] of latest) {
        // The detail view needs fields the event does not carry (sent_at,
        // provider ID), so finished notifications are refetched.
        client.setQueryData<Notification>(keys.notification(id), (n) => (n ? patch(n) : n));
        if (e.status === "sent" || e.status === "failed" || e.status === "retrying") {
          void client.invalidateQueries({ queryKey: keys.notification(id), exact: true });
          void client.invalidateQueries({ queryKey: keys.attempts(id), exact: true });
        }
      }
    }

    if (pendingBatches.size > 0 && !batchTimer) {
      batchTimer = setTimeout(() => {
        batchTimer = undefined;
        for (const id of pendingBatches) {
          void client.invalidateQueries({ queryKey: keys.batch(id), exact: true });
        }
        pendingBatches.clear();
      }, BATCH_REFRESH_MS);
    }
  };
}
