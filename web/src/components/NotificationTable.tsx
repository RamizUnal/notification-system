import type { Notification } from "../api/client";
import { channelInfo, priorityLabel } from "../lib/channels";
import { formatDateTime, relativeTime, shortId, timecode } from "../lib/format";
import { isWaiting } from "../lib/status";
import { StatusMark } from "./StatusMark";
import styles from "./NotificationTable.module.css";

interface NotificationTableProps {
  rows: Notification[];
  selectedId?: string;
  onSelect: (id: string) => void;
}

/**
 * A dense, scannable list of notifications; a row opens its details.
 * Status and attempts come right after the time, so a failure is visible
 * even when the detail panel covers the right side or the screen is narrow.
 */
export function NotificationTable({ rows, selectedId, onSelect }: NotificationTableProps) {
  return (
    <div className={styles.scroller}>
      <table className={styles.table}>
        <thead>
          <tr>
            <th scope="col">Created</th>
            <th scope="col">Status</th>
            <th scope="col" className={styles.right}>Attempts</th>
            <th scope="col">Channel</th>
            <th scope="col">Priority</th>
            <th scope="col">Recipient</th>
            <th scope="col" className={styles.contentCol}>Content</th>
            <th scope="col">ID</th>
            <th scope="col">Due / finished</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((n) => {
            const finished = n.sent_at ?? n.failed_at ?? n.cancelled_at;
            return (
              <tr
                key={n.id}
                aria-selected={n.id === selectedId}
                tabIndex={0}
                onClick={() => onSelect(n.id)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    onSelect(n.id);
                  }
                }}
              >
                <td className={`data ${styles.created}`} title={formatDateTime(n.created_at)}>
                  {timecode(n.created_at)}
                </td>
                <td className={styles.status}>
                  <StatusMark status={n.status} />
                </td>
                <td className={`data ${styles.right} ${styles.attempts}`}>
                  {n.attempts}/{n.max_attempts}
                </td>
                <td className={styles.channel}>{channelInfo[n.channel].label}</td>
                <td className={`${styles.priority} ${n.priority === "high" ? "" : styles.dim}`}>{priorityLabel[n.priority]}</td>
                <td className={`data ${styles.recipient}`}>{n.recipient}</td>
                <td className={styles.contentCol}>
                  <span className={styles.content}>{n.content}</span>
                </td>
                <td className={`data ${styles.dim} ${styles.id}`}>{shortId(n.id)}</td>
                <td className={`data ${styles.dim} ${styles.due}`}>
                  {isWaiting(n.status) && n.next_attempt_at
                    ? relativeTime(n.next_attempt_at)
                    : finished
                      ? timecode(finished, false)
                      : "–"}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
