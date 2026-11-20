import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { Search, X } from "lucide-react";

import type { Channel, Priority, Status } from "../api/client";
import { useStream } from "../api/events";
import { useNotifications, type ListFilter } from "../api/queries";
import { Button } from "../components/Button";
import { Chip } from "../components/Chip";
import { EmptyState } from "../components/EmptyState";
import { inputClass } from "../components/inputClass";
import { NotificationDrawer } from "../components/NotificationDrawer";
import { NotificationTable } from "../components/NotificationTable";
import { ProblemNotice } from "../components/ProblemNotice";
import { Skeleton } from "../components/Skeleton";
import { StatusMark } from "../components/StatusMark";
import { CHANNELS, channelInfo, PRIORITIES, priorityLabel } from "../lib/channels";
import { formatCount, shortId } from "../lib/format";
import { STATUSES } from "../lib/status";
import styles from "./NotificationsPage.module.css";

const ranges = [
  { value: "", label: "Any time" },
  { value: "15m", label: "Last 15 minutes" },
  { value: "1h", label: "Last hour" },
  { value: "24h", label: "Last 24 hours" },
];
const rangeMs: Record<string, number> = { "15m": 15 * 60_000, "1h": 3_600_000, "24h": 86_400_000 };

function listParam<T extends string>(params: URLSearchParams, key: string, allowed: readonly T[]): T[] {
  return (params.get(key) ?? "").split(",").filter((v): v is T => (allowed as readonly string[]).includes(v));
}

export function NotificationsPage() {
  const [params, setParams] = useSearchParams();
  const selected = params.get("id") ?? undefined;

  const filter = useMemo<ListFilter>(() => {
    const f: ListFilter = { limit: 50 };
    const status = listParam<Status>(params, "status", STATUSES);
    const channel = listParam<Channel>(params, "channel", CHANNELS);
    const priority = listParam<Priority>(params, "priority", PRIORITIES);
    if (status.length) f.status = status;
    if (channel.length) f.channel = channel;
    if (priority.length) f.priority = priority;
    if (params.get("recipient")) f.recipient = params.get("recipient") ?? undefined;
    if (params.get("batch_id")) f.batch_id = params.get("batch_id") ?? undefined;
    if (params.get("from")) f.created_from = params.get("from") ?? undefined;
    return f;
  }, [params]);

  const query = useNotifications(filter);
  const rows = useMemo(() => query.data?.pages.flatMap((p) => p.data) ?? [], [query.data]);
  const { log } = useStream();
  const fresh = useMemo(
    () =>
      log
        .filter((e) => e.receivedAt > query.dataUpdatedAt)
        .reduce((n, e) => n + (e.type === "batch.created" ? (e.batchSize ?? 0) : e.type === "notification.created" ? 1 : 0), 0),
    [log, query.dataUpdatedAt],
  );

  const set = (key: string, value: string | null) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: key !== "id" });
  };
  const toggle = (key: string, value: string, current: string[]) => {
    const next = current.includes(value) ? current.filter((v) => v !== value) : [...current, value];
    set(key, next.join(","));
  };
  const active = ["status", "channel", "priority", "recipient", "batch_id", "from"].some((k) => params.get(k));

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <div>
          <h1 className={styles.title}>Notifications</h1>
          <p className={styles.sub}>
            {query.isSuccess ? `${formatCount(rows.length)} shown, newest first` : "Every notification, newest first"}
          </p>
        </div>
        {fresh > 0 && (
          <Button size="sm" onClick={() => void query.refetch()}>
            Show {formatCount(fresh)} new
          </Button>
        )}
      </header>

      <div className={styles.filters} role="group" aria-label="Filters">
        <div className={styles.group}>
          <span className={styles.groupLabel}>Status</span>
          {STATUSES.map((s) => (
            <Chip key={s} pressed={filter.status?.includes(s) ?? false} onClick={() => toggle("status", s, filter.status ?? [])}>
              <StatusMark status={s} />
            </Chip>
          ))}
        </div>
        <div className={styles.row}>
          <div className={styles.group}>
            <span className={styles.groupLabel}>Channel</span>
            {CHANNELS.map((c) => {
              const Icon = channelInfo[c].Icon;
              return (
                <Chip key={c} pressed={filter.channel?.includes(c) ?? false} onClick={() => toggle("channel", c, filter.channel ?? [])}>
                  <Icon aria-hidden="true" /> {channelInfo[c].label}
                </Chip>
              );
            })}
          </div>
          <div className={styles.group}>
            <span className={styles.groupLabel}>Priority</span>
            {PRIORITIES.map((p) => (
              <Chip key={p} pressed={filter.priority?.includes(p) ?? false} onClick={() => toggle("priority", p, filter.priority ?? [])}>
                {priorityLabel[p]}
              </Chip>
            ))}
          </div>
          <label className={styles.inline}>
            <span className="visually-hidden">Created</span>
            <select
              className={`${inputClass} ${styles.select}`}
              value={params.get("range") ?? ""}
              onChange={(e) => {
                const next = new URLSearchParams(params);
                const v = e.target.value;
                if (v) {
                  next.set("range", v);
                  next.set("from", new Date(Date.now() - (rangeMs[v] ?? 0)).toISOString());
                } else {
                  next.delete("range");
                  next.delete("from");
                }
                setParams(next, { replace: true });
              }}
            >
              {ranges.map((r) => (
                <option key={r.value} value={r.value}>
                  {r.label}
                </option>
              ))}
            </select>
          </label>
          <RecipientSearch value={params.get("recipient") ?? ""} onChange={(v) => set("recipient", v)} />
          {params.get("batch_id") && (
            <Chip pressed onClick={() => set("batch_id", null)}>
              Batch {shortId(params.get("batch_id") ?? "")} <X aria-label="Remove batch filter" />
            </Chip>
          )}
          {active && (
            <Button size="sm" variant="ghost" onClick={() => setParams(selected ? { id: selected } : {}, { replace: true })}>
              Clear filters
            </Button>
          )}
        </div>
      </div>

      {query.isError && <ProblemNotice error={query.error} onRetry={() => void query.refetch()} />}
      {query.isPending && (
        <div className={styles.loading} aria-busy="true">
          {Array.from({ length: 8 }, (_, i) => (
            <Skeleton key={i} height={34} />
          ))}
        </div>
      )}
      {query.isSuccess && rows.length === 0 && (
        <EmptyState title={active ? "No notification matches these filters" : "No notifications yet"}>
          {active
            ? "Widen the filters, or clear them to see everything."
            : "Send one from Compose, or fire a burst from the top bar to fill this list."}
        </EmptyState>
      )}
      {rows.length > 0 && (
        <NotificationTable rows={rows} selectedId={selected} onSelect={(id) => set("id", id)} />
      )}
      {query.hasNextPage && (
        <div className={styles.more}>
          <Button busy={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>
            Load more
          </Button>
        </div>
      )}

      {selected && <NotificationDrawer id={selected} onClose={() => set("id", null)} />}
    </div>
  );
}

/** Exact-match recipient search, applied after a short pause in typing. */
function RecipientSearch({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [draft, setDraft] = useState(value);
  useEffect(() => {
    if (draft === value) return;
    const id = setTimeout(() => onChange(draft.trim()), 400);
    return () => clearTimeout(id);
  }, [draft, value, onChange]);
  return (
    <label className={styles.search}>
      <Search aria-hidden="true" />
      <span className="visually-hidden">Recipient</span>
      <input
        className={inputClass}
        type="search"
        placeholder="Exact recipient"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
      />
    </label>
  );
}
