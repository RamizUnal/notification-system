import type { ButtonHTMLAttributes, ReactNode } from "react";

import styles from "./Button.module.css";

type Variant = "primary" | "secondary" | "ghost" | "danger";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: "sm" | "md";
  busy?: boolean;
  icon?: ReactNode;
}

export function Button({ variant = "secondary", size = "md", busy = false, icon, children, disabled, ...rest }: ButtonProps) {
  return (
    <button
      type="button"
      {...rest}
      className={[styles.button, styles[variant], styles[size], rest.className].filter(Boolean).join(" ")}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
    >
      {icon && <span className={styles.icon} aria-hidden="true">{icon}</span>}
      {children}
    </button>
  );
}
