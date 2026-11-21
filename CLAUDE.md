# CLAUDE.md

Guidance for AI coding agents, and for people, working in this repository. The README
covers what the system does; [docs/architecture.md](docs/architecture.md) covers how and
why.

## Commands

```bash
docker compose up --build    # whole stack: console :8070, API :8080, Jaeger :16686, mock provider :8090
make test                    # all Go tests, -race, needs Docker (testcontainers)
make test-unit               # Go unit tests only, no Docker
make test-docker             # all Go tests inside Docker
make test-web                # console: tsc, eslint, vitest
make lint                    # golangci-lint v2 (.golangci.yml)
make generate                # regenerate web/src/api/schema.d.ts from api/openapi.yaml
./scripts/smoke.sh           # end-to-end check against the running stack
```

With Colima instead of Docker Desktop, export these before `make test`:

```bash
export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
```

## Architecture rules

- **Ports and adapters.** `internal/domain` imports nothing of ours. `internal/service`
  (use cases) and `internal/delivery` (the engine) declare the interfaces they need, in
  `ports.go`. Adapters (`storage/postgres`, `provider/webhook`, `ratelimit`, `httpapi`)
  implement them.
- **One composition root.** Only `internal/app` knows the concrete types. New dependencies
  are wired there, and the end-to-end tests reuse those functions.
- **The database is the queue.** Every status change is a conditional update
  (`WHERE status IN (...)`). Completing a delivery is fenced by `lease_id`. Events go out
  with `pg_notify` inside the same transaction. Keep all three when adding transitions, and
  update `domain.Status`'s transition table and its test.
- **Errors.** Domain code returns sentinel or typed errors (`domain.ErrNotFound`,
  `*domain.ValidationError` and the like). Only `internal/httpapi/problem.go` maps them to
  RFC 9457 responses with stable `code` values. Never write an error response anywhere else.

## Conventions

- **Go.** Follow gofmt and goimports, with local imports grouped last
  (`github.com/RamizUnal/notification-system`).
  - Log with `log/slog` and pass the context (`logger.InfoContext(ctx, ...)`), so lines
    carry the correlation and trace IDs.
  - Wrap errors with `%w` and a short description of what failed.
- **Configuration** comes only from environment variables (`internal/config`). Each setting
  gets a default and a check in `validate()`, plus a row in the README's table.
- **Migrations** are numbered pairs in `migrations/` (`000005_name.up.sql` and
  `000005_name.down.sql`). Never edit one that has been committed; add a new one.
  `TestMigrationsRoundTrip` checks up, down, up.
- **API changes.**
  - Update `api/openapi.yaml` in the same change. `TestOpenAPISpecMatchesTheRouter` fails
    on any route the spec does not document, and on any documented route that does not
    exist.
  - Run `make generate` so the console's types follow. CI fails if they differ.
- **Tests.**
  - Unit tests use the fakes in each package's `fakes_test.go` rather than mocks.
  - Anything with a clock runs in a `testing/synctest` bubble.
  - Database and Redis tests are named `*_integration_test.go` and start with
    `//go:build integration`. They take a fresh database from `pgtest.New(t)`.
- **Console** (`web/`):
  - CSS modules and the tokens in `src/styles/tokens.css`.
  - Colour marks state only: on air, probing, circuit open, sent, failure. Channels are
    told apart by labels and line patterns, never by colour.
  - Wire types come from the generated `src/api/schema.d.ts`, so never hand-write API
    types.
  - DESIGN.md records the design system.
- **Commits** follow Conventional Commits (`feat(scope): ...`, `fix`, `test`, `docs`,
  `build`, `ci`, `refactor`, `chore`), and each change ships with its tests.

## Useful facts

- The mock provider rejects recipients ending in `0000`, or containing `invalid`, with
  `422`. Its failure mode can be switched at runtime from the console's provider switch, or
  through `PUT /admin/settings` on port 8090.
- Integration tests run packages one at a time (`-p 1`), because those starting containers
  would otherwise race to start the shared Testcontainers reaper.
- Default limits: 100 messages per second per channel, 5 attempts, batches of up to 1,000,
  scheduling up to 30 days ahead.
