import type { ReactNode } from "react";

import styles from "./Chip.module.css";

/** A toggle for filters: pressed chips are backlit, like a selected router button. */
export function Chip({ pressed, onClick, children }: { pressed: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" className={styles.chip} aria-pressed={pressed} onClick={onClick}>
      {children}
    </button>
  );
}
