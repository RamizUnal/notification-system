import styles from "./LevelMeter.module.css";

interface LevelMeterProps {
  label: string;
  /** Deliveries per second right now. */
  value: number;
  /** Busiest second recently, held as a single lit segment. */
  peak: number;
  /** The per-channel rate limit; the meter's top. */
  ceiling: number;
  segments?: number;
}

/**
 * A segmented level meter, read like an audio meter against its limiter:
 * the column fills with throughput, the top segments light in tally colour
 * only when the channel runs at its ceiling, and a peak-hold segment keeps
 * the busiest recent second visible.
 */
export function LevelMeter({ label, value, peak, ceiling, segments = 20 }: LevelMeterProps) {
  const scale = (v: number) => Math.min(segments, Math.round((v / ceiling) * segments));
  const lit = scale(value);
  const peakIndex = scale(peak) - 1;
  const hotFrom = segments - Math.max(1, Math.round(segments / 10));

  return (
    <div
      className={styles.meter}
      role="meter"
      aria-label={`${label} throughput`}
      aria-valuemin={0}
      aria-valuemax={ceiling}
      aria-valuenow={Math.round(value)}
      aria-valuetext={`${Math.round(value)} per second of a ${ceiling} per second ceiling`}
    >
      <span className={styles.ceiling} aria-hidden="true">
        {ceiling}
      </span>
      <div className={styles.column} aria-hidden="true">
        {Array.from({ length: segments }, (_, i) => {
          const index = segments - 1 - i;
          return (
            <span
              key={index}
              className={styles.segment}
              data-lit={index < lit || undefined}
              data-hot={index >= hotFrom || undefined}
              data-peak={(index === peakIndex && index >= lit) || undefined}
            />
          );
        })}
      </div>
      <span className={styles.floor} aria-hidden="true">
        0
      </span>
    </div>
  );
}
