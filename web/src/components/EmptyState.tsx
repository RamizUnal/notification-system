import type { ReactNode } from "react";

import styles from "./EmptyState.module.css";

/** An empty state that teaches the next step instead of saying "nothing here". */
export function EmptyState({ title, children, action }: { title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className={styles.empty}>
      <p className={styles.title}>{title}</p>
      {children && <div className={styles.body}>{children}</div>}
      {action && <div className={styles.action}>{action}</div>}
    </div>
  );
}
