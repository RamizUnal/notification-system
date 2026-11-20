# 5. Errors are RFC 9457 problem details, mapped in one place

Status: accepted

## Context

Clients need to tell invalid input from conflicts and from outages, fix every invalid field
in one round trip, and quote something to support when a request fails unexpectedly.

## Decision

- The domain defines sentinel errors (`ErrNotFound`, `ErrNotCancellable`, `ErrNotRetryable`,
  `ErrIdempotencyConflict`, `ErrTemplateNameTaken`) and two typed errors carrying details:
  `*ValidationError` (every invalid field, with JSON paths such as
  `notifications[3].recipient`) and `*IdempotencyConflictError` (batch items whose keys are
  taken). Lower layers wrap them with context using `%w`.
- HTTP handlers return errors instead of writing failure responses. One adapter,
  `API.handle`, maps them with `errors.Is` / `errors.As` to `application/problem+json`:
  `type: about:blank`, the status phrase as `title`, a human `detail`, a stable `code`, the
  `request_id` (the correlation ID) and, where relevant, `errors[]` or `conflicts[]`.
- Unknown errors become a 500 with a generic detail; the real error is logged with the
  request's correlation ID and never sent to the client. Panics are recovered the same way.
- The transport adds its own precise errors: malformed JSON (with the byte offset), unknown
  fields, wrong JSON types, empty bodies, trailing data, oversize bodies (413), wrong media
  type (415), invalid query parameters (400, all of them listed).

## Consequences

- Every endpoint fails the same way; clients branch on `code`, which is documented in the
  OpenAPI spec.
- The console shows field errors next to their inputs and the request ID in every error.
