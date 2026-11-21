# Testing

You can run the whole test suite with one command. It takes about half a minute and needs
nothing but Docker. This page covers what is tested at each level, how, and why the suite
is built the way it is.

## Running the tests

| Command | What runs | Needs |
|---|---|---|
| `make test` | Every Go test: unit, integration and end-to-end, with the race detector | Go and a Docker engine |
| `make test-unit` | Unit tests only | Go |
| `make test-integration` | PostgreSQL, Redis and end-to-end tests only | Go and Docker |
| `make test-docker` | Every Go test, inside Docker, against the Compose PostgreSQL and Redis | Docker only |
| `make cover` | Every Go test with coverage; writes `coverage/coverage.html` | Go and Docker |
| `make test-web` | Console: type check, lint, tests | Node 24 |
| `make lint` | golangci-lint with the repository's configuration | golangci-lint |

How the commands are set up:

- **Integration tests sit behind a build tag.** Files named `*_integration_test.go`, and the
  end-to-end suite, have the build tag `integration`. A plain `go test ./...` therefore
  stays fast and needs no Docker. `make test` adds `-tags integration`.
- **Packages run one at a time (`-p 1`).** Without it, the packages that start containers
  race to start the shared Testcontainers reaper. Tests inside a package still run in
  parallel.
- **`make test`, `make test-unit` and `make test-integration` use `-race -count=1`**, so
  concurrency bugs surface and results are never served from the test cache.
- **Colima or Podman** need Testcontainers pointed at their socket first:

  ```bash
  export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock
  export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
  ```

  Docker Desktop needs neither. `make test-docker` avoids the question entirely.

## What each level covers

### Domain rules (unit)

[`internal/domain`](../internal/domain) has no dependencies, so its tests are plain table
tests.

- `TestStatusTransitions` checks the full cross product of statuses against the transition
  table, so adding a transition by accident fails the build.
- The `TestNewNotification*` tests cover per-channel validation: recipient formats and
  length limits per channel, normalisation (trimming the recipient, defaulting the
  priority), and scheduling bounds. One of them checks that every invalid field is
  reported in a single error.
- `TestDraftFingerprint` pins down which fields make two requests "the same" for idempotency.
- The template tests cover rendering, missing variables and batch progress.

### Use cases (unit, with fakes)

[`internal/service`](../internal/service) runs against small in-memory implementations of
its repository ports ([`fakes_test.go`](../internal/service/fakes_test.go)). The tests check
behaviour, not call sequences:

- An idempotent replay returns the original notification, and a different payload under
  the same key is a conflict.
- Invalid input is rejected before anything is stored.
- A batch is all or nothing, and listing pages correctly.
- A manual retry grants a fresh attempt budget.

### Delivery engine (unit, virtual time)

[`internal/delivery`](../internal/delivery) is the most timing-sensitive code, so its tests
run inside [`testing/synctest`](https://pkg.go.dev/testing/synctest) bubbles. Time is
virtual: sleeps and timers complete instantly but in the correct order, so pacing,
cooldowns and shutdown deadlines are asserted exactly rather than with tolerances.

| Test | What it proves |
|---|---|
| `TestDispatcherBoundsConcurrencyAndDrainsTheQueue` | Never more deliveries in flight than configured, and the queue empties |
| `TestDispatcherPacesSendsThroughTheRateLimiter` | Every send waits for a rate-limit slot |
| `TestDispatcherHoldsBackWhileTheCircuitIsOpen` | No claims while the circuit is open, so attempts are not spent |
| `TestDispatcherWaitsForDeliveriesInFlightOnShutdown` | Graceful shutdown finishes started sends |
| `TestDispatcherReturnsClaimsWhenTheLimiterFails` | If Redis fails, claimed work goes back to the queue (fail closed) |
| `TestEngineRecoversLeasesAndReportsHeartbeats` | The engine's background loops run on schedule |
| `TestProcessDecidesWhatHappensAfterAFailure` | Retry, give up or fail permanently, per failure class and attempt count, including `Retry-After` |
| `TestProcessOpensTheCircuitOnlyForProviderFailures` | Provider rejections (`4xx`) do not trip the breaker |
| `TestProcessToleratesALostLease` | A worker that lost its lease records nothing |
| `TestProcessFinishesTheSendDuringShutdown` | A send in progress is never abandoned |
| `TestCircuitBreakerLifecycle` | Closed, then open, then a single half-open probe, then closed or open again |
| `TestExponentialBackoff` | Delays stay within their jittered bounds and respect the cap |

### Provider client (unit)

[`internal/provider/webhook`](../internal/provider/webhook) is tested against `httptest`
servers. The tests cover:

- The documented request: method, JSON body, `Idempotency-Key` and correlation headers.
- Tolerance for unexpected `2xx` bodies.
- The classification table: which status codes are retryable and how `Retry-After` is
  parsed in both its forms.
- Transport errors and timeouts, which count as retryable.

### Rate limiting (unit and integration)

- `TestLocalEnforcesTheRatePerChannel` sends 101 messages at 100 per second and asserts
  that they take exactly one second of virtual time.
- `TestRedisLimitIsSharedByAllWorkers` runs three limiter instances from twelve goroutines
  against one real Redis and checks the combined rate.
- `TestRedisWaitFailsWhenRedisIsDown` checks that the limiter fails closed.

### HTTP API (unit)

[`internal/httpapi`](../internal/httpapi) is exercised through `httptest` with fake services.

- **Every error path has a test.** Malformed JSON, unknown or mistyped fields, empty and
  oversized bodies, form-encoded bodies, bad query parameters, unknown routes and methods,
  and the mapping of each domain error to its status and `code`.
- **Correlation IDs** are accepted from the client or generated, and echoed back.
- **Panics** become a `500` problem with a request ID.
- **WebSocket** tests cover the event stream and its filter validation.
- **`TestOpenAPISpecMatchesTheRouter`** validates [`api/openapi.yaml`](../api/openapi.yaml)
  and compares its paths and methods with the routes chi actually serves, in both
  directions. An undocumented endpoint, or a documented one that does not exist, fails the
  build.

### PostgreSQL (integration)

Each repository runs against a real PostgreSQL 18:

| Test | What it proves |
|---|---|
| `TestCreateIsIdempotentUnderConcurrency` | 20 concurrent creates with one key insert exactly one row |
| `TestConcurrentClaimsNeverShareANotification` | 8 concurrent claimers over 300 rows: every row claimed exactly once |
| `TestClaimHonoursPriorityChannelAndSchedule` | Claims come in priority order, per channel, only when due |
| `TestCompleteRecordsTheOutcomeUnderTheLease` | An outcome is written only with the current lease (fencing) |
| `TestRecoverExpiredLeases` | Crashed workers' notifications are requeued or failed, with the attempt recorded |
| `TestCancelLosesToAClaim` | Cancellation and claiming race safely: exactly one wins |
| `TestBatchOfAThousandIsStoredAtomically` | 1,000 items in one statement, all or nothing |
| `TestListFiltersAndPaginates` | Every filter, and keyset pagination without gaps or repeats |
| `TestMetricsAggregates` | Queue depth, outcome counts, latency percentiles and throughput buckets |
| `TestListenerDeliversCommittedEvents` | `LISTEN/NOTIFY` delivers committed events and never rolled-back ones |
| `TestMigrationsRoundTrip` | Every migration applies, rolls back and applies again |

The schema is migrated once into a template database
([`internal/testutil/pgtest`](../internal/testutil/pgtest)). Each test then gets its own
database through `CREATE DATABASE ... TEMPLATE`, which takes milliseconds, so these tests
run in parallel and never see each other's rows.

### End-to-end

[`test/e2e`](../test/e2e/e2e_test.go) assembles the real system: the API from
`app.BuildAPI` and the delivery engine from `app.BuildEngine`, the same wiring the binaries
use, on a fresh database. Delivery goes to a recording fake provider. It accepts with `202`,
rejects recipients ending in `0000` with `422`, and can fail every first attempt with
`503`. Everything is driven through HTTP:

- `TestBatchIsDeliveredExactlyOncePerNotification`
- `TestTransientFailuresAreRetried`
- `TestPermanentFailuresStopAndCanBeRetriedByHand`
- `TestScheduledNotificationsCanBeCancelled`
- `TestIdempotencyKeyPreventsDuplicates`
- `TestRateLimitHoldsAtTheProvider` measures arrival times at the provider and asserts that
  no rolling second exceeds the limit, allowing one send of timing jitter.
- `TestStatusChangesAreStreamedOverWebSocket`

### Console

The console uses Vitest with Testing Library in jsdom (`web/src/**/*.test.ts(x)`). It covers:

- Formatting, and SMS segment counting (GSM-7 and UCS-2).
- What each channel's tally shows.
- How the plot buckets the metrics series.
- How the API client turns problem details into errors.
- The event stream's batching, rate measurement and resync after a reconnect.
- The level meter's accessible values.
- That the compose form shows server-side validation errors next to the right fields.

## Techniques

- **Real dependencies, started for you or supplied by you.** Integration tests start
  PostgreSQL and Redis with Testcontainers, or use `TEST_DATABASE_URL` and `TEST_REDIS_URL`
  when they are set. CI and `make test-docker` use the latter.
- **Fakes rather than mocks.** Small working implementations of the ports keep tests about
  outcomes, so refactoring the internals does not break them.
- **Virtual time** for anything with a clock, and a clock injected into the services
  (`service.WithClock`) for anything with dates.
- **Contracts checked from both sides.** The server's routes must match the OpenAPI spec.
  The console's TypeScript types are generated from the same spec, and CI fails if the
  committed types differ from a fresh generation.

## Continuous integration

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs four jobs on every push and
pull request:

1. **Lint Go:** golangci-lint.
2. **Test Go:** the unit tests, then the integration and end-to-end tests with coverage.
   Testcontainers starts PostgreSQL and Redis on the runner's Docker engine. The coverage
   report is uploaded as an artifact.
3. **Console:** checks that the API types match the spec, then type check, lint, tests and
   a production build.
4. **One-command setup:** `docker compose up --build --wait`, then
   [`scripts/smoke.sh`](../scripts/smoke.sh), which checks the API and the console, then
   delivers one notification end to end.

## Checking by hand

With the stack running (`docker compose up --build`):

- `./scripts/smoke.sh` checks the API's readiness, the OpenAPI spec, the console and its
  API proxy, then delivers one notification and waits for it to be sent.
- `make demo` sends 1,000 mixed notifications. Watch them drain at 100/s per channel on
  http://localhost:8070, and compare with the rate the mock provider measured at
  http://localhost:8090/admin/stats (`peak_per_second`).
- The console's provider switch (Stable, Flaky, Outage) shows retries, the circuit
  breaker and recovery as they happen. The README's
  [two-minute tour](../README.md#try-it-in-two-minutes) walks through it.
