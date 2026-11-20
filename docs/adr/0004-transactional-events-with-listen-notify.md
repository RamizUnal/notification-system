# 4. Status events are published in the same transaction, over LISTEN/NOTIFY

Status: accepted

## Context

The console shows status changes live, and API replicas must all see changes made by any
worker. Publishing to a broker after committing reintroduces the dual-write problem: a
crash in between loses the event, and publishing before committing can announce a change
that is then rolled back.

## Decision

Every repository method that changes a notification calls `pg_notify('notification_events',
...)` inside its own transaction. PostgreSQL delivers notifications only when the
transaction commits. Each API instance holds one dedicated `LISTEN` connection
(`postgres.Listener`, reconnecting with backoff) and fans events out through
`realtime.Hub` to WebSocket clients, filtered by notification, batch or channel. Batches
publish one `batch.created` event instead of one per item.

## Consequences

- Subscribers never see a change that was rolled back, and nothing extra has to run.
- NOTIFY is fire-and-forget: events committed while a listener reconnects are not replayed.
  The console therefore refetches its data after every reconnect, and the stream is treated
  as a live signal, not the source of truth.
- The hub never blocks publishers: a WebSocket client whose buffer fills up is
  disconnected with close code 4000 so it reconnects and resyncs, instead of silently
  missing events.
- Payloads stay well under NOTIFY's 8 KB limit (error texts are truncated).
