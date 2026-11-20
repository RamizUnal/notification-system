-- Every call to the provider, successful or not. This is the audit trail
-- behind each status change and the source of latency and success metrics.
CREATE TABLE delivery_attempts (
    id                  bigint           GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    notification_id     uuid             NOT NULL REFERENCES notifications (id) ON DELETE CASCADE,
    attempt_number      integer          NOT NULL CHECK (attempt_number > 0),
    -- Denormalized from notifications so metrics never need a join.
    channel             text             NOT NULL CHECK (channel IN ('sms', 'email', 'push')),
    outcome             text             NOT NULL CHECK (outcome IN ('success', 'retryable_failure', 'permanent_failure')),
    status_code         integer,
    error               text,
    provider_message_id text,
    duration_ms         double precision NOT NULL CHECK (duration_ms >= 0),
    started_at          timestamptz      NOT NULL,
    finished_at         timestamptz      NOT NULL,

    CONSTRAINT delivery_attempts_number_key UNIQUE (notification_id, attempt_number)
);

-- Metrics read a sliding window of recent attempts.
CREATE INDEX delivery_attempts_finished_idx ON delivery_attempts (finished_at DESC);
