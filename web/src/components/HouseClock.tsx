import { useEffect, useState } from "react";

import { timecode } from "../lib/format";
import styles from "./HouseClock.module.css";

/** The gallery's master clock: local time, the reference for every timecode on screen. */
export function HouseClock() {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  return (
    <time className={styles.clock} dateTime={new Date(now).toISOString()} aria-label="Local time">
      {timecode(now, false)}
    </time>
  );
}
