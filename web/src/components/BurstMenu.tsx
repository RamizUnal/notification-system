import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { ChevronDown, Radio } from "lucide-react";

import { useCreateBatch } from "../api/queries";
import { burstPresets, type BurstPreset } from "../lib/demo";
import { formatCount, shortId } from "../lib/format";
import { Button } from "./Button";
import { ProblemNotice } from "./ProblemNotice";
import { useToast } from "./Toasts";
import styles from "./BurstMenu.module.css";

/** Fires a burst of synthetic notifications, the quickest way to see the engine work. */
export function BurstMenu() {
  const [open, setOpen] = useState(false);
  const wrapper = useRef<HTMLDivElement>(null);
  const create = useCreateBatch();
  const toast = useToast();

  useEffect(() => {
    if (!open) return;
    const onPointer = (e: PointerEvent) => {
      if (!wrapper.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const fire = (preset: BurstPreset) => {
    create.mutate(
      { items: preset.build() },
      {
        onSuccess: (batch) => {
          setOpen(false);
          toast({
            tone: "live",
            title: `Burst queued: ${formatCount(batch.size)} notifications`,
            body: (
              <Link to={`/batches/${batch.id}`}>Follow batch {shortId(batch.id)}</Link>
            ),
          });
        },
      },
    );
  };

  return (
    <div className={styles.wrap} ref={wrapper}>
      <Button
        variant="primary"
        icon={<Radio />}
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-haspopup="menu"
        busy={create.isPending}
      >
        <span className={styles.long}>Send burst</span>
        <span className={styles.short}>Burst</span>
        <ChevronDown size={14} aria-hidden="true" />
      </Button>
      {open && (
        <div className={styles.menu} role="menu" aria-label="Burst presets">
          <p className={styles.note}>Synthetic notifications, labelled as demos.</p>
          {burstPresets.map((p) => (
            <button key={p.id} type="button" role="menuitem" className={styles.item} onClick={() => fire(p)} disabled={create.isPending}>
              <span className={styles.label}>{p.label}</span>
              <span className={styles.description}>{p.description}</span>
            </button>
          ))}
          <Link to="/compose?tab=batch" role="menuitem" className={styles.custom} onClick={() => setOpen(false)}>
            Build a custom batch…
          </Link>
          {create.isError && <ProblemNotice error={create.error} />}
        </div>
      )}
    </div>
  );
}
