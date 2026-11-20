# 2. A shared per-channel rate limit with GCRA in Redis

Status: accepted

## Context

The provider accepts at most 100 messages per second per channel. Workers scale
horizontally (`WORKER_REPLICAS`), so a limit kept in each process would let N workers send
N times the limit.

## Decision

Workers pace every send through `delivery.Limiter`. In Docker Compose the implementation is
`ratelimit.Redis`: the generic cell rate algorithm (via `go-redis/redis_rate`), evaluated
atomically in Redis with Redis' own clock, one key per channel. Burst is 1 by default, so
sends are spaced evenly (one every 10 ms at 100/s) and no rolling second exceeds the limit.
`ratelimit.Local` (token buckets from `x/time/rate`) implements the same interface for
single-process runs and tests.

The dispatcher claims only as many notifications as it has free delivery slots and then
waits for a rate-limit slot per notification before handing it to a delivery goroutine. A
claimed notification is therefore sent within moments, which keeps leases short and
respects priority at claim time.

## Consequences

- The limit holds across every worker process (integration test: three limiter instances
  with four goroutines each never exceed the limit in any rolling second).
- Channels have separate budgets: a busy SMS channel never slows email.
- If Redis is unreachable, `Wait` fails and the channel pauses: claimed notifications go
  back to the queue. Exceeding the provider's limit would get messages throttled or
  rejected; pausing only delays them. This is a deliberate fail-closed choice.
- The limit applies when sends start. The provider can see a few more arrivals within one
  second when network or provider latency varies (the mock provider's 30-150 ms jitter
  shows this as an occasional 101).
