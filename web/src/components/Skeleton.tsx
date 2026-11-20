import styles from "./Skeleton.module.css";

/** A placeholder block shown while data loads, sized like what it stands in for. */
export function Skeleton({ width = "100%", height = 14 }: { width?: number | string; height?: number | string }) {
  return <span className={styles.skeleton} style={{ width, height }} aria-hidden="true" />;
}
