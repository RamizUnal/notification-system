import type { ReactNode } from "react";

import styles from "./SegmentedControl.module.css";

interface Option<T extends string> {
  value: T;
  label: ReactNode;
  disabled?: boolean;
}

interface SegmentedControlProps<T extends string> {
  label: string;
  value: T;
  options: readonly Option<T>[];
  onChange: (value: T) => void;
  size?: "sm" | "md";
  hideLabel?: boolean;
}

/** A row of mutually exclusive choices, like the lit buttons of a router panel. */
export function SegmentedControl<T extends string>({
  label,
  value,
  options,
  onChange,
  size = "md",
  hideLabel = true,
}: SegmentedControlProps<T>) {
  return (
    <div className={styles.wrap}>
      <span className={hideLabel ? "visually-hidden" : styles.label} id={`seg-${label}`}>
        {label}
      </span>
      <div role="radiogroup" aria-labelledby={`seg-${label}`} className={`${styles.group} ${styles[size]}`}>
        {options.map((o) => (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={o.value === value}
            disabled={o.disabled}
            className={styles.segment}
            onClick={() => onChange(o.value)}
          >
            {o.label}
          </button>
        ))}
      </div>
    </div>
  );
}
