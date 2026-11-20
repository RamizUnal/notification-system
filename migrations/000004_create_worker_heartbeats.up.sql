-- Workers report liveness and per-channel circuit breaker state every few
-- seconds, so the API and the console can show the delivery engine's health
-- without talking to each worker.
CREATE TABLE worker_heartbeats (
    worker_id  text        PRIMARY KEY,
    started_at timestamptz NOT NULL,
    seen_at    timestamptz NOT NULL,
    in_flight  integer     NOT NULL DEFAULT 0 CHECK (in_flight >= 0),
    -- {"sms": {"state": "open", "retry_at": "2026-11-20T12:00:00Z"}, ...}
    circuits   jsonb       NOT NULL DEFAULT '{}'::jsonb
);
