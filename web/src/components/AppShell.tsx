import { useEffect } from "react";
import { NavLink, Outlet } from "react-router";
import { useQueryClient } from "@tanstack/react-query";

import { eventStream } from "../api/events";
import { createEventApplier } from "../api/liveCache";
import { BurstMenu } from "./BurstMenu";
import { ProviderSwitch } from "./ProviderSwitch";
import { StreamTally } from "./StreamTally";
import { ToastProvider } from "./Toasts";
import styles from "./AppShell.module.css";

const nav = [
  { to: "/", label: "Overview", end: true },
  { to: "/notifications", label: "Notifications" },
  { to: "/compose", label: "Compose" },
  { to: "/templates", label: "Templates" },
  { to: "/provider", label: "Provider" },
];

/** Keeps the query cache in step with the event stream and resyncs after reconnects. */
function LiveSync() {
  const client = useQueryClient();
  useEffect(() => {
    eventStream.start();
    const offEvents = eventStream.onEvents(createEventApplier(client));
    const offResync = eventStream.onResync(() => void client.invalidateQueries());
    return () => {
      offEvents();
      offResync();
    };
  }, [client]);
  return null;
}

export function AppShell() {
  // Toasts live inside the router so their links can navigate.
  return (
    <ToastProvider>
      <LiveSync />
      <a href="#main" className={styles.skip}>
        Skip to content
      </a>
      <header className={styles.bar}>
        <NavLink to="/" className={styles.brand} aria-label="Dispatch, notification operations: overview">
          <span className={styles.mark} aria-hidden="true" />
          <span className={styles.name}>Dispatch</span>
          <span className={styles.sub}>notification operations</span>
        </NavLink>
        <nav className={styles.nav} aria-label="Main">
          {nav.map((n) => (
            <NavLink key={n.to} to={n.to} end={n.end} className={styles.link}>
              {n.label}
            </NavLink>
          ))}
        </nav>
        <div className={styles.controls}>
          <StreamTally />
          <ProviderSwitch />
          <BurstMenu />
        </div>
      </header>
      <main id="main" className={styles.main}>
        <Outlet />
      </main>
    </ToastProvider>
  );
}
