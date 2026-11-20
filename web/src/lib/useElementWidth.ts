import { useEffect, useRef, useState } from "react";

/** Tracks an element's content width, for drawing SVG at real pixel size. */
export function useElementWidth<T extends HTMLElement>(fallback = 600) {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(fallback);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setWidth(Math.max(240, Math.floor(entry.contentRect.width)));
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  return [ref, width] as const;
}
