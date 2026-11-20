import { useState } from "react";
import { Check, Copy } from "lucide-react";

import styles from "./CopyButton.module.css";

export function CopyButton({ value, label = "Copy" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className={styles.copy}
      onClick={() => {
        void navigator.clipboard?.writeText(value).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
      aria-label={copied ? "Copied" : `${label} ${value}`}
      title={copied ? "Copied" : label}
    >
      {copied ? <Check size={14} /> : <Copy size={14} />}
    </button>
  );
}
