# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

React + TypeScript + Vite single-page app in `web/`. API types are generated from the
OpenAPI spec (`api/openapi.yaml`), server state is managed with TanStack Query, tests run on
Vitest. In Docker Compose the built app is served by nginx, which also proxies the API and
WebSocket. Chosen by the user.

## Users

Primary: on-call and operations engineers who own notification delivery. They watch queue
health and throughput, investigate failed sends, cancel or retry notifications, and fire test
bursts to see rate limiting and retries behave. They use the console during normal operation
and during incidents such as flash-sale or breaking-news traffic spikes.

Secondary: Insider One engineers reviewing this assessment, who use the console to verify that
the engine does what the brief asks. API consumers integrate through the REST API and its
OpenAPI documentation rather than the console.

## Product Purpose

An event-driven notification system that accepts SMS, email and push notifications over a REST
API, queues them by priority, and delivers them asynchronously through an external provider
(simulated with webhook.site or the bundled mock provider). The console gives real-time
visibility into and control over that pipeline.

Success: an operator can tell at a glance whether delivery is healthy, find any notification
and its full delivery history in seconds, and act on it (cancel, retry) safely.

## Positioning

The console shows the delivery pipeline's real mechanics (priority queues, the per-channel
100 msg/s ceiling, retry backoff, provider circuit state, every delivery attempt) instead of
vanity totals. Everything on screen is read from the live system.

## Operating Context

- Started locally with one command (`docker compose up`); the console, API, worker, database
  and mock provider run together.
- Reviewers open the console, send single notifications, batches and bursts, and watch queue
  depth drain at the rate limit while statuses update live over WebSocket.
- On-call use: triage during traffic spikes and provider outages; drill from an aggregate
  signal (failure rate, queue depth) to the specific notification and its attempt timeline.

## Capabilities and Constraints

- Channels: `sms`, `email`, `push`. Priorities: `high`, `normal`, `low`.
- Create single notifications; create batches of up to 1,000 notifications per request.
- Query by notification ID or batch ID; list with filters (status, channel, priority, date
  range) and cursor pagination.
- Cancel notifications that are still waiting (scheduled, queued or awaiting retry).
- Asynchronous processing by queue workers; at most 100 messages per second per channel.
- Content validation with per-channel character limits and recipient formats.
- Idempotency keys prevent duplicate notifications and duplicate sends.
- Retries with backoff for transient provider failures; permanent failures are not retried.
- Scheduled delivery, message templates with variable substitution, real-time status updates
  over WebSocket, metrics (queue depth, success/failure rates, latency), health checks.
- Undecided / out of scope: authentication and multi-tenancy (not part of the assessment).

## Brand Commitments

Presented as an internal Insider One tool: it uses Insider One's brand colors as seen in the
supplied culture deck (orange-red on deep aubergine / near-black, with warm off-white). It does
not use the Insider One logo and does not claim to be an official Insider One product.

## Evidence on Hand

- The assessment brief ("Software Engineer Assessment - Golang") and the Insider One culture
  deck, both supplied by the user (not committed to the repository).
- No real customer data, traffic figures, benchmarks or testimonials. Every notification the
  console shows is synthetic and created by the person running it; nothing may be fabricated.

## Product Principles

1. Truth over decoration: every number and status comes from the running system.
2. Triage first: the shortest path runs from "something is wrong" to the exact notification
   and its attempt history.
3. Safe actions: cancel, retry and bursts state their scope and are hard to trigger by accident.
4. Show the mechanism: rate limits, priorities, retries and circuit state stay visible, because
   the engine is what is being evaluated.
