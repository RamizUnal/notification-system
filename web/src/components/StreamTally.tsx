import { useStream } from "../api/events";
import { Lamp } from "./StatusMark";
import styles from "./StreamTally.module.css";

const view = {
  live: { tone: "live", label: "Live", hint: "Receiving status changes as they happen" },
  connecting: { tone: "muted", label: "Connecting", hint: "Opening the event stream" },
  reconnecting: { tone: "probe", label: "Reconnecting", hint: "Lost the event stream; retrying and refreshing data" },
} as const;

/** The gallery's on-air light for the event stream itself. */
export function StreamTally() {
  const { status } = useStream();
  const v = view[status];
  return (
    <span className={styles.tally} title={v.hint}>
      <Lamp tone={v.tone} label={v.label} />
    </span>
  );
}
