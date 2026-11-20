import { useState } from "react";
import { useSearchParams } from "react-router";

import type { Channel, Metrics } from "../api/client";
import { useLiveRates, useStream } from "../api/events";
import { useMetrics } from "../api/queries";
import { AsRunLog } from "../components/AsRunLog";
import { ChannelMonitor } from "../components/ChannelMonitor";
import { EmptyState } from "../components/EmptyState";
import { NotificationDrawer } from "../components/NotificationDrawer";
import { ProblemNotice } from "../components/ProblemNotice";
import { SegmentedControl } from "../components/SegmentedControl";
import { Skeleton } from "../components/Skeleton";
import { ThroughputPlot } from "../components/ThroughputPlot";
import { CHANNELS, channelInfo } from "../lib/channels";
import { formatCount } from "../lib/format";
import styles from "./OverviewPage.module.css";

const windows = [
  { value: "1m", label: "1 min" },
  { value: "5m", label: "5 min" },
  { value: "15m", label: "15 min" },
  { value: "1h", label: "1 hour" },
] as const;
type WindowValue = (typeof windows)[number]["value"];

export function OverviewPage() {
  const [params, setParams] = useSearchParams();
  const windowValue = (windows.find((w) => w.value === params.get("window"))?.value ?? "5m") as WindowValue;
  const windowLabel = windows.find((w) => w.value === windowValue)?.label ?? "5 min";
  const selected = params.get("id");
  const [focus, setFocus] = useState<Channel | null>(null);

  const metrics = useMetrics(windowValue);
  const live = useLiveRates();
  const { log } = useStream();
  const m = metrics.data;
  const ceiling = m?.channels[0]?.rate_limit_per_second ?? 100;

  const update = (key: string, value: string | null) => {
    const next = new URLSearchParams(params);
    if (value === null) next.delete(key);
    else next.set(key, value);
    setParams(next, { replace: true });
  };

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <div className={styles.titleBlock}>
          <h1 className={styles.title}>Overview</h1>
          {m ? <Summary metrics={m} windowLabel={windowLabel} /> : <Skeleton width={420} height={18} />}
        </div>
        <SegmentedControl
          label="Time window"
          value={windowValue}
          options={windows}
          onChange={(w) => update("window", w === "5m" ? null : w)}
        />
      </header>

      {metrics.isError && (
        <ProblemNotice
          title="Metrics are unavailable"
          error={metrics.error}
          onRetry={() => void metrics.refetch()}
        />
      )}

      <div className={styles.wall}>
        {CHANNELS.map((ch) => (
          <ChannelMonitor
            key={ch}
            channel={ch}
            metrics={m?.channels.find((c) => c.channel === ch)}
            live={live[ch]}
            workers={m?.workers ?? []}
            windowLabel={windowLabel}
            focused={focus === ch}
            dimmed={focus !== null && focus !== ch}
            onFocus={() => setFocus(focus === ch ? null : ch)}
          />
        ))}
      </div>

      <div className={styles.lower}>
        <section className={styles.panel} aria-labelledby="plot-title">
          <header className={styles.panelHead}>
            <h2 id="plot-title" className={styles.panelTitle}>
              Deliveries per second
            </h2>
            <ul className={styles.legend} aria-label="Line styles">
              {CHANNELS.map((ch) => (
                <li key={ch} data-channel={ch}>
                  <svg width="22" height="8" aria-hidden="true">
                    <line x1="1" x2="21" y1="4" y2="4" />
                  </svg>
                  {channelInfo[ch].label}
                </li>
              ))}
            </ul>
          </header>
          <div className={styles.plot}>
            {m ? <ThroughputPlot metrics={m} ceiling={ceiling} focus={focus} /> : <Skeleton height={250} />}
          </div>
        </section>

        <AsRunLog
          events={log}
          channel={focus}
          onSelect={(id) => update("id", id)}
          empty={
            <EmptyState title="Nothing has gone out yet">
              Use <strong>Send burst</strong> in the top bar: deliveries appear here with their timecodes as they go
              out, and every line opens the notification behind it.
            </EmptyState>
          }
        />
      </div>

      {selected && <NotificationDrawer id={selected} onClose={() => update("id", null)} />}
    </div>
  );
}

function Summary({ metrics, windowLabel }: { metrics: Metrics; windowLabel: string }) {
  const sum = (pick: (c: Metrics["channels"][number]) => number) =>
    metrics.channels.reduce((total, c) => total + pick(c), 0);
  const waiting = sum((c) => Object.values(c.queue.by_priority).reduce((a, b) => a + b, 0));
  const failedAttempts = sum((c) => c.deliveries.retryable_failures + c.deliveries.permanent_failures);
  const workers = metrics.workers.length;

  return (
    <p className={styles.summary}>
      Last {windowLabel}: <b className="num">{formatCount(sum((c) => c.deliveries.succeeded))}</b> sent ·{" "}
      <b className="num" data-alarm={failedAttempts > 0 || undefined}>{formatCount(failedAttempts)}</b> failed attempts ·{" "}
      <b className="num">{formatCount(waiting)}</b> waiting now ·{" "}
      <b className="num" data-alarm={workers === 0 || undefined}>{workers}</b> {workers === 1 ? "worker" : "workers"} on duty
    </p>
  );
}
