# 1. PostgreSQL is both the system of record and the queue

Status: accepted

## Context

Notifications must never be lost once the API has answered `202 Accepted`, must be
delivered in priority order, must support scheduled delivery, retries with backoff and
cancellation of anything not yet in flight, and must survive bursts (flash sales,
breaking news). Throughput is bounded by the provider contract: at most 100 messages per
second per channel, so 300 per second in total.

The usual design pairs a database with a broker (Kafka, RabbitMQ, SQS, Redis lists). That
creates a dual write: the row is committed but the enqueue fails (or the other way round),
and an outbox table plus a relay process is needed to make the pair reliable again.

## Decision

The `notifications` table is the queue. Workers claim due rows with

```sql
WITH due AS (
  SELECT id FROM notifications
  WHERE channel = $1 AND status IN ('scheduled','queued','retrying') AND next_attempt_at <= $2
  ORDER BY priority_rank, next_attempt_at
  LIMIT $3
  FOR UPDATE SKIP LOCKED
)
UPDATE notifications ... SET status = 'processing', lease_id = gen_random_uuid(), ...
```

backed by a partial index on `(channel, priority_rank, next_attempt_at)` that only holds
waiting rows. The queue is reached through the `delivery.Store` port, so the engine does not
know it is PostgreSQL.

## Consequences

- Accepting a notification is one atomic write; there is no window in which it exists in
  one system and not the other, and no outbox or relay to operate.
- Priorities, scheduling (`next_attempt_at` in the future), retries (move
  `next_attempt_at`) and cancellation (a conditional `UPDATE ... WHERE status IN (...)`)
  are all plain SQL on one row, protected by row locks.
- `SKIP LOCKED` lets any number of workers claim concurrently without blocking each other
  or double-claiming (verified by an integration test with 8 concurrent claimers).
- The partial index keeps claims cheap however large the history grows. At the contract's
  300 msg/s ceiling PostgreSQL is far from its limits; a queue depth query on millions of
  waiting rows becomes slow, which would be the first thing to replace with counters.
- Strict priority can starve low priority under sustained high-priority load. That is what
  the brief asks for; weighted fair dequeueing would be a change inside the claim query.
- If throughput requirements outgrow one PostgreSQL primary, a broker-backed adapter can
  implement `delivery.Store` (with an outbox for the dual write) without touching the engine.
