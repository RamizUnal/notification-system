import type { Status } from "../api/client";
import { statusInfo, type Tone } from "../lib/status";
import styles from "./StatusMark.module.css";

/** A tally lamp: a square mark whose colour is a state, always with its name. */
export function Lamp({ tone, label, title }: { tone: Tone; label?: string; title?: string }) {
  return (
    <span className={styles.mark} data-tone={tone} title={title}>
      <span className={styles.lamp} aria-hidden="true" />
      {label && <span className={styles.label}>{label}</span>}
    </span>
  );
}

export function StatusMark({ status }: { status: Status }) {
  const info = statusInfo[status];
  return <Lamp tone={info.tone} label={info.label} title={info.hint} />;
}
