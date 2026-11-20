# 3. At-least-once delivery with leases, classified retries and a circuit breaker

Status: accepted

## Context

A worker can die between sending a message and recording the result, a provider can fail
transiently or permanently, and during an outage blind retries would burn every
notification's attempt budget against a dead endpoint.

## Decision

**Leases.** A claim sets `status = processing`, a random `lease_id` and `lease_expires_at`.
Recording the outcome is an `UPDATE ... WHERE id = $1 AND lease_id = $2`; a worker whose lease
expired gets `domain.ErrLeaseLost` and cannot overwrite the worker that took over. Every
worker periodically recovers expired leases: the notification goes back to `retrying`, or
to `failed` if that was its last attempt, and the lost attempt is written to the audit log
as "outcome unknown".

**Classification.** The provider client turns responses into `provider.Error` values:
timeouts, network errors, `408`, `425`, `429` and `5xx` are retryable; any other `4xx` is
permanent and is not retried. A `2xx` with an unexpected body still counts as accepted,
because resending would duplicate the message.

**Backoff.** Retries wait `ExponentialBackoff` with equal jitter: a random duration between
half and all of `min(Max, Base * 2^(n-1))`, so notifications that failed together do not
return together, and a retry never fires instantly. A provider's `Retry-After` wins when it
is longer (capped at one hour). After `max_attempts` the notification is `failed`, which is
the dead-letter state: queryable, filterable, and re-queued with `POST /retry`, which grants
a fresh attempt budget.

**Circuit breaker.** Each channel has a breaker per worker: after N consecutive provider
failures it opens, the dispatcher stops claiming, and after a cooldown a single probe is let
through; its result closes or reopens the circuit. Rejections (4xx) count as successes for the
breaker: they prove the provider is up. Workers publish their breaker state in heartbeats, so
the API and the console can show it.

**Duplicate suppression towards the provider.** Every request carries
`Idempotency-Key: <notification id>`. Exactly-once delivery is impossible end to end; with a
provider that honours idempotency keys, at-least-once becomes effectively once.

## Consequences

- No notification is lost to a crash: at worst it is retried (and the provider sees its key
  again).
- Permanent errors fail fast and visibly instead of looping.
- During an outage, notifications wait in the queue instead of exhausting their attempts,
  and the provider gets room to recover.
- Each channel is a bulkhead: its own dispatcher, pool, rate limit and breaker, so a broken
  SMS provider does not slow email or push.
