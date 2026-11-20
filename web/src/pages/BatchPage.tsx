import { useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { Ban } from "lucide-react";

import { useBatch, useCancelBatch, useNotifications } from "../api/queries";
import { BatchProgress } from "../components/BatchProgress";
import { Button } from "../components/Button";
import { CopyButton } from "../components/CopyButton";
import { NotificationDrawer } from "../components/NotificationDrawer";
import { NotificationTable } from "../components/NotificationTable";
import { ProblemNotice } from "../components/ProblemNotice";
import { Skeleton } from "../components/Skeleton";
import { useToast } from "../components/Toasts";
import { formatCount, formatDateTime, shortId } from "../lib/format";
import styles from "./BatchPage.module.css";

export function BatchPage() {
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const selected = params.get("id") ?? undefined;
  const batch = useBatch(id);
  const list = useNotifications(useMemo(() => ({ batch_id: id, limit: 50 }), [id]));
  const rows = useMemo(() => list.data?.pages.flatMap((p) => p.data) ?? [], [list.data]);
  const cancel = useCancelBatch();
  const [confirming, setConfirming] = useState(false);
  const toast = useToast();

  const b = batch.data;
  const waiting = b ? (b.status_counts.scheduled ?? 0) + (b.status_counts.queued ?? 0) + (b.status_counts.retrying ?? 0) : 0;

  return (
    <div className={styles.page}>
      <nav className={styles.crumbs} aria-label="Breadcrumb">
        <Link to="/notifications">Notifications</Link> / Batch
      </nav>
      <header className={styles.head}>
        <div>
          <h1 className={styles.title}>
            Batch <span className="data">{shortId(id)}</span>
            <CopyButton value={id} label="Copy batch ID" />
          </h1>
          {b ? (
            <p className={styles.sub}>
              {formatCount(b.size)} notifications · created {formatDateTime(b.created_at)} ·{" "}
              {b.done ? "finished" : `${formatCount(b.pending)} still in progress`}
            </p>
          ) : (
            <Skeleton width={320} />
          )}
        </div>
        {b && waiting > 0 && (
          <div className={styles.cancel}>
            {confirming ? (
              <>
                <span>Cancel {formatCount(waiting)} waiting notifications?</span>
                <Button
                  size="sm"
                  variant="danger"
                  busy={cancel.isPending}
                  onClick={() =>
                    cancel.mutate(id, {
                      onSuccess: (res) => {
                        setConfirming(false);
                        toast({ tone: "muted", title: `Cancelled ${formatCount(res.cancelled)} notifications` });
                      },
                    })
                  }
                >
                  Yes, cancel them
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>
                  Keep them
                </Button>
              </>
            ) : (
              <Button variant="danger" icon={<Ban />} onClick={() => setConfirming(true)}>
                Cancel waiting
              </Button>
            )}
          </div>
        )}
      </header>

      {batch.isError && <ProblemNotice error={batch.error} onRetry={() => void batch.refetch()} />}
      {cancel.isError && <ProblemNotice error={cancel.error} />}
      {b && (
        <section className={styles.panel} aria-label="Progress">
          <BatchProgress batch={b} />
        </section>
      )}

      {list.isError && <ProblemNotice error={list.error} onRetry={() => void list.refetch()} />}
      {rows.length > 0 && (
        <NotificationTable
          rows={rows}
          selectedId={selected}
          onSelect={(nid) => setParams({ id: nid })}
        />
      )}
      {list.hasNextPage && (
        <div className={styles.more}>
          <Button busy={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
            Load more
          </Button>
        </div>
      )}
      {selected && <NotificationDrawer id={selected} onClose={() => setParams({})} />}
    </div>
  );
}
