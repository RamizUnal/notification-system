import { useId, type ReactNode } from "react";

import styles from "./Field.module.css";

interface FieldProps {
  label: string;
  hint?: ReactNode;
  error?: string;
  aside?: ReactNode;
  children: (props: { id: string; "aria-describedby"?: string; "aria-invalid"?: boolean }) => ReactNode;
}

/** A labelled control with its hint or error, wired for assistive technology. */
export function Field({ label, hint, error, aside, children }: FieldProps) {
  const id = useId();
  const noteId = `${id}-note`;
  const note = error ?? hint;
  return (
    <div className={styles.field}>
      <div className={styles.top}>
        <label htmlFor={id} className={styles.label}>
          {label}
        </label>
        {aside && <span className={styles.aside}>{aside}</span>}
      </div>
      {children({ id, "aria-describedby": note ? noteId : undefined, "aria-invalid": error ? true : undefined })}
      {note && (
        <p id={noteId} className={error ? styles.error : styles.hint}>
          {note}
        </p>
      )}
    </div>
  );
}
