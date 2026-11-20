import type { Batch, Status } from "../api/client";
import { formatCount } from "../lib/format";
import { statusInfo } from "../lib/status";
import { StatusMark } from "./StatusMark";
import styles from "./BatchProgress.module.css";

const order: Status[] = ["sent", "failed", "cancelled", "processing", "retrying", "queued", "scheduled"];

/** A batch's notifications as one bar, segmented by status. */
export function BatchProgress({ batch }: { batch: Batch }) {
  const total = Math.max(1, batch.size);
  const counts = order.map((s) => [s, batch.status_counts[s] ?? 0] as const);
  return (
    <div className={styles.progress}>
      <div
        className={styles.bar}
        role="img"
        aria-label={counts.filter(([, n]) => n > 0).map(([s, n]) => `${formatCount(n)} ${statusInfo[s].label.toLowerCase()}`).join(", ")}
      >
        {counts.map(([s, n]) =>
          n > 0 ? <span key={s} data-status={s} style={{ width: `${(n / total) * 100}%` }} title={`${statusInfo[s].label}: ${formatCount(n)}`} /> : null,
        )}
      </div>
      <ul className={styles.legend}>
        {counts.map(([s, n]) => (
          <li key={s} data-empty={n === 0 || undefined}>
            <StatusMark status={s} />
            <span className="data">{formatCount(n)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
