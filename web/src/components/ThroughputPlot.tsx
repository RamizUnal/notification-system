import { useMemo, useState } from "react";

import type { Channel, Metrics } from "../api/client";
import { CHANNELS, channelInfo } from "../lib/channels";
import { formatRate, timecode } from "../lib/format";
import { toBuckets } from "../lib/plot";
import { useElementWidth } from "../lib/useElementWidth";
import styles from "./ThroughputPlot.module.css";

const HEIGHT = 250;
const PAD = { top: 14, right: 84, bottom: 50, left: 40 };
const FAILURE_BAND = 16;

interface ThroughputPlotProps {
  metrics: Metrics;
  ceiling: number;
  focus: Channel | null;
}

/**
 * Deliveries per second per channel across the window. Channels are told
 * apart by stroke pattern and direct labels, not colour: colour stays
 * reserved for state, here the failure band and nothing else.
 */
export function ThroughputPlot({ metrics, ceiling, focus }: ThroughputPlotProps) {
  const [ref, width] = useElementWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const buckets = useMemo(() => toBuckets(metrics), [metrics]);

  const plotW = width - PAD.left - PAD.right;
  const plotH = HEIGHT - PAD.top - PAD.bottom;
  const maxRate = Math.max(ceiling * 1.15, ...buckets.flatMap((b) => CHANNELS.map((c) => b.rates[c] * 1.1)));
  const maxFailed = Math.max(1, ...buckets.map((b) => b.failed));
  const first = buckets[0]?.at ?? 0;
  const span = Math.max(1, (buckets.at(-1)?.at ?? 1) - first);

  const x = (t: number) => PAD.left + ((t - first) / span) * plotW;
  const y = (rate: number) => PAD.top + plotH - (rate / maxRate) * plotH;
  const baseY = PAD.top + plotH;
  const bandTop = baseY + 24;
  const step = buckets.length > 1 ? plotW / (buckets.length - 1) : plotW;

  const ticks = useMemo(() => timeTicks(buckets, metrics.window_seconds), [buckets, metrics.window_seconds]);
  const last = buckets.at(-1);
  const labelY = spreadLabels(
    CHANNELS.map((ch) => [ch, y(last?.rates[ch] ?? 0)] as const),
    13,
    PAD.top + 6,
    baseY,
  );
  const hovered = hover !== null ? buckets[hover] : undefined;

  if (buckets.length < 2) {
    return (
      <div ref={ref} className={styles.wrap}>
        <p className={styles.waiting}>Collecting the first data points…</p>
      </div>
    );
  }

  return (
    <div ref={ref} className={styles.wrap}>
      <svg
        width={width}
        height={HEIGHT}
        role="img"
        aria-label={`Deliveries per second per channel over the last ${Math.round(metrics.window_seconds / 60)} minutes`}
        onMouseLeave={() => setHover(null)}
        onMouseMove={(e) => {
          const box = e.currentTarget.getBoundingClientRect();
          const i = Math.round((e.clientX - box.left - PAD.left) / step);
          setHover(i >= 0 && i < buckets.length ? i : null);
        }}
      >
        {[0, ceiling / 2].map((v) => (
          <g key={v}>
            <line className={styles.grid} x1={PAD.left} x2={PAD.left + plotW} y1={y(v)} y2={y(v)} />
            <text className={styles.axis} x={PAD.left - 8} y={y(v) + 4} textAnchor="end">
              {v}
            </text>
          </g>
        ))}

        <line className={styles.ceiling} x1={PAD.left} x2={PAD.left + plotW} y1={y(ceiling)} y2={y(ceiling)} />
        <text className={styles.ceilingLabel} x={PAD.left - 8} y={y(ceiling) + 4} textAnchor="end">
          {ceiling}
        </text>
        <text className={styles.ceilingLabel} x={PAD.left + 6} y={y(ceiling) - 6}>
          ceiling {ceiling}/s per channel
        </text>

        {ticks.map((t) => (
          <text key={t} className={styles.axis} x={x(t)} y={baseY + 16} textAnchor="middle">
            {timecode(t, false).slice(0, 5)}
          </text>
        ))}

        {CHANNELS.map((ch) => {
          const d = buckets.map((b, i) => `${i === 0 ? "M" : "L"}${x(b.at).toFixed(1)},${y(b.rates[ch]).toFixed(1)}`).join("");
          return (
            <g key={ch} className={styles.series} data-channel={ch} data-dimmed={(focus !== null && focus !== ch) || undefined}>
              <path d={d} />
              {last && (
                <text x={PAD.left + plotW + 8} y={(labelY[ch] ?? 0) + 4}>
                  {channelInfo[ch].label} {formatRate(last.rates[ch])}
                </text>
              )}
            </g>
          );
        })}

        <text className={styles.axis} x={PAD.left - 8} y={bandTop + FAILURE_BAND - 3} textAnchor="end">
          fail
        </text>
        <line className={styles.grid} x1={PAD.left} x2={PAD.left + plotW} y1={bandTop + FAILURE_BAND} y2={bandTop + FAILURE_BAND} />
        {buckets.map((b) =>
          b.failed > 0 ? (
            <rect
              key={b.at}
              className={styles.failure}
              x={x(b.at) - Math.max(1.5, step * 0.3)}
              width={Math.max(3, step * 0.6)}
              y={bandTop + FAILURE_BAND - (b.failed / maxFailed) * FAILURE_BAND}
              height={(b.failed / maxFailed) * FAILURE_BAND}
            />
          ) : null,
        )}

        {hovered && (
          <line className={styles.cursor} x1={x(hovered.at)} x2={x(hovered.at)} y1={PAD.top} y2={bandTop + FAILURE_BAND} />
        )}
      </svg>

      {hovered && (
        <div
          className={styles.readout}
          style={{ left: Math.min(width - 190, Math.max(0, x(hovered.at) + 12)) }}
          aria-hidden="true"
        >
          <p className="data">{timecode(hovered.at, false)}</p>
          {CHANNELS.map((ch) => (
            <p key={ch}>
              <span>{channelInfo[ch].label}</span>
              <span className="data">{formatRate(hovered.rates[ch])}</span>
            </p>
          ))}
          <p data-alarm={hovered.failed > 0 || undefined}>
            <span>Failed attempts</span>
            <span className="data">{hovered.failed}</span>
          </p>
        </div>
      )}
    </div>
  );
}

/**
 * Moves direct labels apart so none overlap: sorted by height, each label
 * keeps at least `gap` pixels from the previous one, within [min, max].
 */
function spreadLabels(wanted: readonly (readonly [Channel, number])[], gap: number, min: number, max: number) {
  const sorted = [...wanted].sort((a, b) => a[1] - b[1]);
  const out: Partial<Record<Channel, number>> = {};
  let previous = -Infinity;
  for (const [ch, target] of sorted) {
    const placed = Math.max(target, previous + gap, min);
    out[ch] = placed;
    previous = placed;
  }
  // If the stack ran past the bottom, shift it back up as a block.
  const overflow = previous - max;
  if (overflow > 0) for (const [ch] of sorted) out[ch] = (out[ch] ?? 0) - overflow;
  return out;
}

/** Picks round-minute (or round-second) ticks that fit the window. */
function timeTicks(buckets: { at: number }[], windowSeconds: number): number[] {
  if (buckets.length === 0) return [];
  const stepMs = (windowSeconds <= 120 ? 15 : windowSeconds <= 600 ? 60 : windowSeconds <= 3600 ? 600 : 3600) * 1000;
  const start = Math.ceil((buckets[0]?.at ?? 0) / stepMs) * stepMs;
  const end = buckets.at(-1)?.at ?? 0;
  const out: number[] = [];
  for (let t = start; t <= end; t += stepMs) out.push(t);
  return out;
}
