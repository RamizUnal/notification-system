import { useEffect, useState, useSyncExternalStore } from "react";

import type { Channel, Priority, Status } from "./client";

/** One committed change, as received from /api/v1/events. */
export interface LiveEvent {
  seq: number;
  type: "notification.created" | "notification.updated" | "batch.created";
  notificationId?: string;
  batchId?: string;
  channel?: Channel;
  priority?: Priority;
  status?: Status;
  attempts: number;
  lastError?: string;
  batchSize?: number;
  occurredAt: string;
  receivedAt: number;
}

export type StreamStatus = "connecting" | "live" | "reconnecting";

interface Snapshot {
  status: StreamStatus;
  /** Newest first, at most LOG_LIMIT entries. */
  log: readonly LiveEvent[];
}

interface WireMessage {
  type: string;
  data?: {
    notification_id?: string;
    batch_id?: string;
    channel?: Channel;
    priority?: Priority;
    status?: Status;
    attempts?: number;
    last_error?: string;
    batch_size?: number;
    occurred_at?: string;
  };
}

const LOG_LIMIT = 300;
const FLUSH_INTERVAL_MS = 120;
const RATE_WINDOW_MS = 1000;
const PEAK_HOLD_MS = 3000;
const RESYNC_CLOSE_CODE = 4000;

type EventsHandler = (events: LiveEvent[]) => void;

/**
 * EventStream keeps one WebSocket to the API and turns its messages into
 * observable state. Messages are buffered and flushed in batches, so a
 * burst of hundreds of status changes per second costs a few renders, not
 * hundreds. After a reconnect, resync handlers run, because events
 * committed while disconnected are not replayed.
 */
export class EventStream {
  private socket?: WebSocket;
  private snapshot: Snapshot = { status: "connecting", log: [] };
  private readonly listeners = new Set<() => void>();
  private readonly handlers = new Set<EventsHandler>();
  private readonly resyncHandlers = new Set<() => void>();
  private pending: LiveEvent[] = [];
  private flushTimer?: ReturnType<typeof setTimeout>;
  private reconnectTimer?: ReturnType<typeof setTimeout>;
  private retryDelay = 1000;
  private seq = 0;
  private everConnected = false;
  private started = false;
  private readonly sentAt: Record<Channel, number[]> = { sms: [], email: [], push: [] };

  constructor(
    private readonly url: () => string,
    private readonly createSocket: (url: string) => WebSocket = (u) => new WebSocket(u),
  ) {}

  start(): void {
    if (this.started) return;
    this.started = true;
    this.connect();
  }

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): Snapshot => this.snapshot;

  /** Registers a handler for every flushed batch of events. */
  onEvents(handler: EventsHandler): () => void {
    this.handlers.add(handler);
    return () => this.handlers.delete(handler);
  }

  /** Registers a handler that runs after the stream reconnects. */
  onResync(handler: () => void): () => void {
    this.resyncHandlers.add(handler);
    return () => this.resyncHandlers.delete(handler);
  }

  /** Deliveries accepted per second on a channel, over the last second. */
  rate(channel: Channel, now = Date.now()): number {
    return this.sentAt[channel].filter((t) => now - t < RATE_WINDOW_MS).length;
  }

  /** The busiest rolling second on a channel within the peak-hold window. */
  peak(channel: Channel, now = Date.now()): number {
    const times = this.sentAt[channel].filter((t) => now - t < PEAK_HOLD_MS);
    let best = 0;
    let end = 0;
    for (let start = 0; start < times.length; start++) {
      while (end < times.length && (times[end] ?? 0) - (times[start] ?? 0) < RATE_WINDOW_MS) end++;
      best = Math.max(best, end - start);
    }
    return best;
  }

  private connect(): void {
    const socket = this.createSocket(this.url());
    this.socket = socket;

    socket.onopen = () => {
      this.retryDelay = 1000;
      const isReconnect = this.everConnected;
      this.everConnected = true;
      this.setStatus("live");
      if (isReconnect) this.resyncHandlers.forEach((h) => h());
    };
    socket.onmessage = (msg) => this.receive(msg.data);
    socket.onclose = (ev) => {
      this.socket = undefined;
      this.setStatus("reconnecting");
      const delay = ev.code === RESYNC_CLOSE_CODE ? 0 : this.retryDelay;
      this.retryDelay = Math.min(this.retryDelay * 2, 10_000);
      this.reconnectTimer = setTimeout(() => this.connect(), delay);
    };
    socket.onerror = () => socket.close();
  }

  private receive(raw: unknown): void {
    if (typeof raw !== "string") return;
    let msg: WireMessage;
    try {
      msg = JSON.parse(raw) as WireMessage;
    } catch {
      return;
    }
    if (msg.type === "ready" || !msg.data) return;

    const d = msg.data;
    const event: LiveEvent = {
      seq: ++this.seq,
      type: msg.type as LiveEvent["type"],
      notificationId: d.notification_id,
      batchId: d.batch_id,
      channel: d.channel,
      priority: d.priority,
      status: d.status,
      attempts: d.attempts ?? 0,
      lastError: d.last_error,
      batchSize: d.batch_size,
      occurredAt: d.occurred_at ?? new Date().toISOString(),
      receivedAt: Date.now(),
    };
    if (event.status === "sent" && event.channel) {
      this.sentAt[event.channel].push(event.receivedAt);
    }
    this.pending.push(event);
    this.flushTimer ??= setTimeout(() => this.flush(), FLUSH_INTERVAL_MS);
  }

  private flush(): void {
    this.flushTimer = undefined;
    const batch = this.pending;
    this.pending = [];
    if (batch.length === 0) return;

    const now = Date.now();
    for (const ch of Object.keys(this.sentAt) as Channel[]) {
      this.sentAt[ch] = this.sentAt[ch].filter((t) => now - t < PEAK_HOLD_MS);
    }
    const newestFirst = [...batch].reverse();
    this.snapshot = { ...this.snapshot, log: [...newestFirst, ...this.snapshot.log].slice(0, LOG_LIMIT) };
    this.handlers.forEach((h) => h(batch));
    this.emit();
  }

  private setStatus(status: StreamStatus): void {
    if (this.snapshot.status === status) return;
    this.snapshot = { ...this.snapshot, status };
    this.emit();
  }

  private emit(): void {
    this.listeners.forEach((l) => l());
  }

  /** Stops the stream; used by tests. */
  stop(): void {
    clearTimeout(this.flushTimer);
    clearTimeout(this.reconnectTimer);
    if (this.socket) {
      this.socket.onclose = null;
      this.socket.close();
    }
  }
}

function eventsURL(): string {
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${window.location.host}/api/v1/events`;
}

export const eventStream = new EventStream(eventsURL);

export function useStream(): Snapshot {
  return useSyncExternalStore(eventStream.subscribe, eventStream.getSnapshot);
}

export interface LiveRate {
  /** Deliveries in the last second. */
  rate: number;
  /** Busiest rolling second in the last three seconds (peak hold). */
  peak: number;
}

const idleRates: Record<Channel, LiveRate> = {
  sms: { rate: 0, peak: 0 },
  email: { rate: 0, peak: 0 },
  push: { rate: 0, peak: 0 },
};

/**
 * useLiveRates reports deliveries per second per channel, measured from the
 * event stream and recomputed every quarter second, so a meter falls back to
 * zero when its channel goes quiet.
 */
export function useLiveRates(): Record<Channel, LiveRate> {
  const [rates, setRates] = useState(idleRates);
  useEffect(() => {
    const id = setInterval(() => {
      const now = Date.now();
      const measure = (ch: Channel): LiveRate => ({ rate: eventStream.rate(ch, now), peak: eventStream.peak(ch, now) });
      setRates({ sms: measure("sms"), email: measure("email"), push: measure("push") });
    }, 250);
    return () => clearInterval(id);
  }, []);
  return rates;
}
