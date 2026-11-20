import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import { X } from "lucide-react";

import type { Tone } from "../lib/status";
import { Lamp } from "./StatusMark";
import styles from "./Toasts.module.css";

interface Toast {
  id: number;
  tone: Tone;
  title: string;
  body?: ReactNode;
}

type Notify = (toast: Omit<Toast, "id">) => void;

const ToastContext = createContext<Notify>(() => {});

/** Brief, polite confirmations of actions; errors that need a decision stay inline. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const next = useRef(0);

  const dismiss = useCallback((id: number) => setToasts((all) => all.filter((t) => t.id !== id)), []);

  const notify = useCallback<Notify>(
    (toast) => {
      const id = ++next.current;
      setToasts((all) => [...all.slice(-3), { ...toast, id }]);
      setTimeout(() => dismiss(id), 6000);
    },
    [dismiss],
  );

  const value = useMemo(() => notify, [notify]);
  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className={styles.region} role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={styles.toast}>
            <Lamp tone={t.tone} />
            <div className={styles.text}>
              <p className={styles.title}>{t.title}</p>
              {t.body && <div className={styles.body}>{t.body}</div>}
            </div>
            <button type="button" className={styles.close} onClick={() => dismiss(t.id)} aria-label="Dismiss">
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useToast(): Notify {
  return useContext(ToastContext);
}
