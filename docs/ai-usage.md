# How AI was used

This project was built with [Claude Code](https://claude.com/claude-code), Anthropic's
coding agent, running Claude Opus 5.5. The agent wrote the code, the tests and the
documentation, working from the assessment brief and from decisions the developer made
along the way. Every commit carries a
`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` trailer, so the history shows
where the agent contributed.

This page records how the work was divided, how the output was checked, and where that
checking changed the result.

## What the developer decided

Questions with real trade-offs were put to the developer as explicit choices before any
work depending on them started:

| Question | Decision |
|---|---|
| Language and setup | Go, with everything runnable through Docker Compose |
| Version control | Small conventional commits as each piece is finished; nothing pushed by the agent |
| Who the console is for | On-call and operations engineers |
| How the console presents itself | An internal Insider One tool, in the brand palette, with no logo and no official claims |
| Console stack | React, TypeScript and Vite |
| The console's first screen | The live pipeline overview |
| Demo tools in the console | A burst sender, a provider failure switch, links to traces |
| What to avoid | A generic admin template, a marketing look, screens too dense to read |
| Visual direction | "Broadcast Gallery": a broadcast control room, picked from several proposed directions |

The engineering decisions that followed (PostgreSQL as the queue, GCRA in Redis,
at-least-once delivery with leases, `LISTEN/NOTIFY` events, problem details for errors) are
recorded with their alternatives in the [decision records](adr). They are open to review
like any other design choice.

## How the work was done

1. **Brief first.** The assessment brief and the evaluation criteria (Docker, migrations,
   design patterns, code quality, tests, UI, error handling, API design) were turned into a
   plan and an architecture before any code was written.
2. **Inside out.** The domain model and its rules came first, then the use cases and the
   ports they need, then the adapters (PostgreSQL, Redis, HTTP, the provider client), then
   the wiring. Each layer came with its tests in the same commit, so the history reads as
   the system being built.
3. **The console as a design project.** The operator console was built with the
   Impeccable design skill:
   - A product brief ([PRODUCT.md](../PRODUCT.md)) and a written direction contract
     ([`.impeccable/surfaces`](../.impeccable/surfaces)) came before any UI code.
   - The finished screens were checked by an automated detector for common design defects.
   - A separate reviewer agent scored screenshots of the running console against the
     contract. It saw the screens but not the reasoning behind them.
   - The design system the console actually ships is recorded in [DESIGN.md](../DESIGN.md).

## How the output was checked

Nothing was accepted because it looked plausible. Each change had to get past:

- **the compiler and linters:** golangci-lint for Go, which includes `go vet`; `tsc` in
  strict mode and ESLint for the console;
- **the tests**, with the race detector, at every level from domain rules to end-to-end
  ([testing.md](testing.md));
- **the running system:** the stack started with `docker compose up`, bursts of 1,000
  notifications sent through it, and the rate limit confirmed by the rate the mock provider
  itself measured, not only by the system's own metrics;
- **for the console:** screenshots of the real screens at desktop and phone widths, judged
  by the reviewer agent until it reported nothing left to fix.

## Where checking changed the result

These came out of the checks above, not out of the first draft:

- **A test that hung.** The provider client's timeout test blocked forever. The test
  server's shutdown waited for a handler that was itself waiting for the test to finish.
  Reordering the test's cleanup fixed it without weakening what it checks.
- **Configuration that accepted nothing.** A required setting was satisfied by an empty
  string, so a blank `DATABASE_URL` slipped through. It is now rejected at startup.
- **A spec that parsed differently than it read.** Commas inside unquoted OpenAPI
  descriptions split them into separate YAML keys. The spec's own validation test caught it.
- **An import cycle** between the delivery engine and the metrics code was resolved by
  moving Prometheus into its own package, which also made the dependency rule cleaner.
- **Short IDs that were all the same.** The console first showed the start of each UUID,
  but UUIDv7 IDs start with a timestamp, so a burst of notifications all looked alike. The
  reviewer spotted it in a screenshot, and short IDs now use the random tail.
- **A plot that disagreed with its own meters.** During a burst the throughput plot's end
  labels read about half of what the live meters showed. The values came from a snapshot
  that trails real time by a few seconds. The plot now ends on the same live reading the
  meters use.
- **A health check that failed only in the container:** `localhost` resolved to IPv6 inside
  the nginx image, so the console's health check now uses `127.0.0.1`.

## Limits

- The agent did not push to any remote, create accounts or use credentials. The bundled
  mock provider made it possible to test failure modes without depending on webhook.site.
- An AI reviewer is a useful second reader, not a substitute for a human one. The decision
  records and this page exist so that a human reviewer can check the reasoning, not just
  the result.
- [CLAUDE.md](../CLAUDE.md) gives the commands and conventions to any agent, or person,
  continuing the work.
