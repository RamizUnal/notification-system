import { useEffect, useRef } from "react";
import { Link } from "react-router";
import { Ban, ExternalLink, RotateCcw, X } from "lucide-react";

import type { Attempt, Notification } from "../api/client";
import { useAttempts, useCancelNotification, useNotification, useRetryNotification } from "../api/queries";
import { channelInfo, priorityLabel } from "../lib/channels";
import { traceLink } from "../lib/config";
import { formatDateTime, formatMs, relativeTime, shortId, timecode } from "../lib/format";
import { isWaiting } from "../lib/status";
import { Button } from "./Button";
import { CopyButton } from "./CopyButton";
import { ProblemNotice } from "./ProblemNotice";
import { Skeleton } from "./Skeleton";
import { Lamp, StatusMark } from "./StatusMark";
import { useToast } from "./Toasts";
import styles from "./NotificationDrawer.module.css";

interface NotificationDrawerProps {
  id: string;
  onClose: () => void;
}

/**
 * The detail panel for one notification: its message, lifecycle, every
 * delivery attempt, and the actions its status allows. It sits beside the
 * list rather than over it, so the list stays usable while it is open.
 */
export function NotificationDrawer({ id, onClose }: NotificationDrawerProps) {
  const heading = useRef<HTMLHeadingElement>(null);
  const notification = useNotification(id);
  const attempts = useAttempts(id);

  useEffect(() => {
    heading.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [id, onClose]);

  const n = notification.data;
  return (
    <aside className={styles.drawer} aria-labelledby="drawer-title">
      <header className={styles.head}>
        <h2 id="drawer-title" ref={heading} tabIndex={-1} className={styles.title}>
          Notification <span className="data">{shortId(id)}</span>
          <CopyButton value={id} label="Copy ID" />
        </h2>
        <button type="button" className={styles.close} onClick={onClose} aria-label="Close details">
          <X size={18} />
        </button>
      </header>

      <div className={styles.body}>
        {notification.isPending && <DrawerSkeleton />}
        {notification.isError && <ProblemNotice error={notification.error} onRetry={() => void notification.refetch()} />}
        {n && (
          <>
            <Actions n={n} />
            <Message n={n} />
            <Lifecycle n={n} />
            <section className={styles.section} aria-labelledby="attempts-title">
              <h3 id="attempts-title" className={styles.sectionTitle}>
                Delivery attempts <span className={styles.count}>{n.attempts} of {n.max_attempts}</span>
              </h3>
              {attempts.isPending && <Skeleton height={48} />}
              {attempts.isError && <ProblemNotice error={attempts.error} onRetry={() => void attempts.refetch()} />}
              {attempts.data && <AttemptTimeline attempts={attempts.data.data} />}
            </section>
          </>
        )}
      </div>
    </aside>
  );
}

function Actions({ n }: { n: Notification }) {
  const cancel = useCancelNotification();
  const retry = useRetryNotification();
  const toast = useToast();
  const error = cancel.error ?? retry.error;

  return (
    <div className={styles.actions}>
      <div className={styles.status}>
        <StatusMark status={n.status} />
        {isWaiting(n.status) && n.next_attempt_at && (
          <span className={styles.next}>due {relativeTime(n.next_attempt_at)}</span>
        )}
      </div>
      <div className={styles.buttons}>
        {isWaiting(n.status) && (
          <Button
            size="sm"
            variant="danger"
            icon={<Ban />}
            busy={cancel.isPending}
            onClick={() =>
              cancel.mutate(n.id, { onSuccess: () => toast({ tone: "muted", title: "Notification cancelled" }) })
            }
          >
            Cancel delivery
          </Button>
        )}
        {n.status === "failed" && (
          <Button
            size="sm"
            icon={<RotateCcw />}
            busy={retry.isPending}
            onClick={() =>
              retry.mutate(n.id, {
                onSuccess: (r) => toast({ tone: "neutral", title: "Back in the queue", body: `${r.max_attempts - r.attempts} more attempts allowed` }),
              })
            }
          >
            Retry
          </Button>
        )}
        {n.trace_id && (
          <a className={styles.trace} href={traceLink(n.trace_id)} target="_blank" rel="noreferrer">
            <ExternalLink size={14} aria-hidden="true" /> Open trace
          </a>
        )}
      </div>
      {error && <ProblemNotice error={error} />}
    </div>
  );
}

function Message({ n }: { n: Notification }) {
  const info = channelInfo[n.channel];
  return (
    <section className={styles.section} aria-labelledby="message-title">
      <h3 id="message-title" className={styles.sectionTitle}>
        Message
      </h3>
      <dl className={styles.facts}>
        <div>
          <dt>Channel</dt>
          <dd>
            <info.Icon size={14} aria-hidden="true" /> {info.label}
          </dd>
        </div>
        <div>
          <dt>Priority</dt>
          <dd>{priorityLabel[n.priority]}</dd>
        </div>
        <div className={styles.wide}>
          <dt>Recipient</dt>
          <dd className="data">{n.recipient}</dd>
        </div>
      </dl>
      <p className={styles.content}>{n.content}</p>
    </section>
  );
}

function Lifecycle({ n }: { n: Notification }) {
  const rows: [string, React.ReactNode][] = [
    ["Created", formatDateTime(n.created_at)],
    ...(n.scheduled_at ? [["Scheduled for", formatDateTime(n.scheduled_at)] as [string, string]] : []),
    ...(n.sent_at ? [["Sent", formatDateTime(n.sent_at)] as [string, string]] : []),
    ...(n.failed_at ? [["Failed", formatDateTime(n.failed_at)] as [string, string]] : []),
    ...(n.cancelled_at ? [["Cancelled", formatDateTime(n.cancelled_at)] as [string, string]] : []),
    ...(n.provider_message_id ? [["Provider message", <span className="data">{n.provider_message_id}</span>] as [string, React.ReactNode]] : []),
    ...(n.idempotency_key ? [["Idempotency key", <span className="data">{n.idempotency_key}</span>] as [string, React.ReactNode]] : []),
    ...(n.correlation_id ? [["Correlation ID", <span className="data">{n.correlation_id}</span>] as [string, React.ReactNode]] : []),
    ...(n.batch_id ? [["Batch", <Link to={`/batches/${n.batch_id}`} className="data">{shortId(n.batch_id)}</Link>] as [string, React.ReactNode]] : []),
  ];
  return (
    <section className={styles.section} aria-labelledby="lifecycle-title">
      <h3 id="lifecycle-title" className={styles.sectionTitle}>
        Lifecycle
      </h3>
      {n.last_error && (
        <p className={styles.lastError}>
          <Lamp tone="alarm" /> {n.last_error}
        </p>
      )}
      <dl className={styles.list}>
        {rows.map(([term, value]) => (
          <div key={term}>
            <dt>{term}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

const outcomeTone = { success: "clear", retryable_failure: "probe", permanent_failure: "alarm" } as const;
const outcomeLabel = { success: "Accepted", retryable_failure: "Transient failure", permanent_failure: "Rejected" } as const;

function AttemptTimeline({ attempts }: { attempts: Attempt[] }) {
  if (attempts.length === 0) {
    return <p className={styles.none}>No attempt yet. The notification is waiting for a worker.</p>;
  }
  return (
    <ol className={styles.timeline}>
      {attempts.map((a) => (
        <li key={a.number}>
          <div className={styles.attemptHead}>
            <span className="data">#{a.number}</span>
            <Lamp tone={outcomeTone[a.outcome]} label={outcomeLabel[a.outcome]} />
            <span className={styles.meta}>
              {a.status_code !== null && a.status_code !== undefined && <span className="data">HTTP {a.status_code}</span>}
              <span className="data">{formatMs(a.duration_ms)}</span>
              <span className="data">{timecode(a.finished_at)}</span>
            </span>
          </div>
          {a.error && <p className={styles.attemptError}>{a.error}</p>}
        </li>
      ))}
    </ol>
  );
}

function DrawerSkeleton() {
  return (
    <div className={styles.section}>
      <Skeleton width="40%" height={20} />
      <Skeleton height={60} />
      <Skeleton width="70%" />
      <Skeleton width="55%" />
    </div>
  );
}
