import { useState } from "react";
import { Link } from "react-router";
import { Pause, Play } from "lucide-react";

import type { Channel } from "../api/client";
import type { LiveEvent } from "../api/events";
import { channelInfo } from "../lib/channels";
import { formatCount, shortId, timecode } from "../lib/format";
import { Button } from "./Button";
import { StatusMark } from "./StatusMark";
import styles from "./AsRunLog.module.css";

interface AsRunLogProps {
  events: readonly LiveEvent[];
  channel: Channel | null;
  onSelect: (notificationId: string) => void;
  empty: React.ReactNode;
}

/**
 * The as-run log: what actually went out, newest first, with timecodes.
 * Pausing freezes the list so an operator can read it during a burst.
 */
export function AsRunLog({ events, channel, onSelect, empty }: AsRunLogProps) {
  const [frozen, setFrozen] = useState<readonly LiveEvent[] | null>(null);
  const shown = (frozen ?? events).filter((e) => !channel || !e.channel || e.channel === channel);
  const missed = frozen ? events.length - frozen.length : 0;

  return (
    <section className={styles.log} aria-labelledby="as-run-title">
      <header className={styles.head}>
        <h2 id="as-run-title" className={styles.title}>
          As-run log{channel && <span className={styles.scope}> · {channelInfo[channel].label}</span>}
        </h2>
        <Button
          size="sm"
          variant="ghost"
          icon={frozen ? <Play /> : <Pause />}
          onClick={() => setFrozen(frozen ? null : events)}
          aria-pressed={frozen !== null}
        >
          {frozen ? (missed > 0 ? `Resume (${formatCount(missed)} new)` : "Resume") : "Pause"}
        </Button>
      </header>

      {shown.length === 0 ? (
        <div className={styles.empty}>{empty}</div>
      ) : (
        <ol className={styles.rows}>
          {shown.slice(0, 150).map((e) => (
            <li key={e.seq}>
              {e.type === "batch.created" ? (
                <Link className={styles.row} to={`/batches/${e.batchId ?? ""}`}>
                  <span className={`${styles.time} data`}>{timecode(e.occurredAt)}</span>
                  <span className={styles.batch}>Batch of {formatCount(e.batchSize ?? 0)} queued</span>
                  <span className={`${styles.id} data`}>{shortId(e.batchId ?? "")}</span>
                </Link>
              ) : (
                <button type="button" className={styles.row} onClick={() => e.notificationId && onSelect(e.notificationId)}>
                  <span className={`${styles.time} data`}>{timecode(e.occurredAt)}</span>
                  <span className={`${styles.channel} data`}>{e.channel ? channelInfo[e.channel].label : "–"}</span>
                  <span className={styles.status}>{e.status && <StatusMark status={e.status} />}</span>
                  <span className={`${styles.id} data`}>
                    {shortId(e.notificationId ?? "")}
                    {e.attempts > 1 && <span className={styles.attempt}> #{e.attempts}</span>}
                  </span>
                </button>
              )}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
