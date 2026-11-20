import type { Channel, ChannelMetrics, Worker } from "../api/client";
import type { LiveRate } from "../api/events";
import { channelInfo, PRIORITIES, priorityLabel } from "../lib/channels";
import { formatCount, formatMs, formatPercent } from "../lib/format";
import { tallyFor } from "../lib/tally";
import { Skeleton } from "./Skeleton";
import { LevelMeter } from "./LevelMeter";
import styles from "./ChannelMonitor.module.css";

interface ChannelMonitorProps {
  channel: Channel;
  metrics?: ChannelMetrics;
  live: LiveRate;
  workers: Worker[];
  windowLabel: string;
  focused: boolean;
  dimmed: boolean;
  onFocus: () => void;
}

export function ChannelMonitor({ channel, metrics, live, workers, windowLabel, focused, dimmed, onFocus }: ChannelMonitorProps) {
  const info = channelInfo[channel];
  const tally = metrics ? tallyFor(metrics, live, workers) : undefined;
  const waiting = metrics ? PRIORITIES.reduce((sum, p) => sum + (metrics.queue.by_priority[p] ?? 0), 0) : 0;
  const largest = metrics ? Math.max(1, ...PRIORITIES.map((p) => metrics.queue.by_priority[p] ?? 0)) : 1;
  const d = metrics?.deliveries;
  const failures = d ? d.retryable_failures + d.permanent_failures : 0;

  return (
    <section
      className={styles.monitor}
      data-tally={tally?.state ?? "idle"}
      data-focused={focused || undefined}
      data-dimmed={dimmed || undefined}
      aria-labelledby={`monitor-${channel}`}
    >
      <div className={styles.screen}>
        <div className={styles.meterCol}>
          <LevelMeter label={info.label} value={live.rate} peak={live.peak} ceiling={metrics?.rate_limit_per_second ?? 100} />
          <p className={styles.rate}>
            <span className="data">{Math.round(live.rate)}</span>
            <span className={styles.unit}>/s</span>
          </p>
        </div>

        {metrics && d ? (
          <div className={styles.readouts}>
            <div className={styles.block}>
              <div className={styles.row}>
                <span className={styles.term}>Waiting</span>
                <span className={`${styles.big} data`}>{formatCount(waiting)}</span>
              </div>
              <p className={styles.split}>
                <span className="num">{formatCount(metrics.queue.due)}</span> due ·{" "}
                <span className="num">{formatCount(metrics.queue.scheduled)}</span> scheduled ·{" "}
                <span className="num">{formatCount(metrics.queue.retrying)}</span> retrying ·{" "}
                <span className="num">{formatCount(metrics.queue.processing)}</span> in flight
              </p>
              <ul className={styles.priorities} aria-label="Waiting by priority">
                {PRIORITIES.map((p) => {
                  const n = metrics.queue.by_priority[p] ?? 0;
                  return (
                    <li key={p}>
                      <span className={styles.pLabel}>{priorityLabel[p]}</span>
                      <span className={styles.track}>
                        <span className={styles.fill} style={{ "--share": n / largest } as React.CSSProperties} />
                      </span>
                      <span className={`${styles.pCount} data`}>{formatCount(n)}</span>
                    </li>
                  );
                })}
              </ul>
            </div>

            <dl className={styles.stats}>
              <div>
                <dt>Sent, {windowLabel}</dt>
                <dd className="data">{formatCount(d.succeeded)}</dd>
              </div>
              <div>
                <dt>Failed attempts</dt>
                <dd className="data" data-alarm={failures > 0 || undefined}>
                  {formatCount(failures)}
                </dd>
              </div>
              <div>
                <dt>Success</dt>
                <dd className="data">{formatPercent(d.success_rate)}</dd>
              </div>
              <div>
                <dt>Latency p50 · p95 · p99</dt>
                <dd className="data">
                  {d.success_rate === null
                    ? "–"
                    : `${formatMs(d.latency_ms.p50)} · ${formatMs(d.latency_ms.p95)} · ${formatMs(d.latency_ms.p99)}`}
                </dd>
              </div>
            </dl>
          </div>
        ) : (
          <div className={styles.readouts} aria-busy="true">
            <Skeleton width="60%" height={22} />
            <Skeleton width="90%" />
            <Skeleton height={44} />
            <Skeleton width="80%" />
          </div>
        )}
      </div>

      <button type="button" className={styles.umd} onClick={onFocus} aria-pressed={focused}>
        <span className={styles.name} id={`monitor-${channel}`}>
          <info.Icon aria-hidden="true" />
          {info.label}
        </span>
        <span className={styles.state}>
          <span className={styles.lamp} aria-hidden="true" />
          <span className={styles.stateLabel}>{tally?.label ?? "Loading"}</span>
          {tally && <span className={styles.detail}>{tally.detail}</span>}
        </span>
      </button>
    </section>
  );
}
