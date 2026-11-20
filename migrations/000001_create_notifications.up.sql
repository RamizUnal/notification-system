-- Batches group the notifications created by one batch request.
CREATE TABLE batches (
    id              uuid        PRIMARY KEY,
    size            integer     NOT NULL CHECK (size BETWEEN 1 AND 1000),
    idempotency_key text,
    fingerprint     text        NOT NULL,
    created_at      timestamptz NOT NULL
);

CREATE UNIQUE INDEX batches_idempotency_key_key
    ON batches (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Notifications are both the system of record and the work queue. Workers
-- claim due rows with SELECT ... FOR UPDATE SKIP LOCKED, so accepting a
-- notification and enqueueing it is one atomic write: there is no second
-- system that could miss a message after the API has acknowledged it.
CREATE TABLE notifications (
    id                  uuid        PRIMARY KEY,
    batch_id            uuid        REFERENCES batches (id) ON DELETE CASCADE,
    channel             text        NOT NULL CHECK (channel IN ('sms', 'email', 'push')),
    priority            text        NOT NULL CHECK (priority IN ('high', 'normal', 'low')),
    priority_rank       smallint    NOT NULL GENERATED ALWAYS AS (
                            CASE priority WHEN 'high' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END
                        ) STORED,
    recipient           text        NOT NULL,
    content             text        NOT NULL,
    status              text        NOT NULL CHECK (status IN (
                            'scheduled', 'queued', 'processing', 'retrying', 'sent', 'failed', 'cancelled'
                        )),
    idempotency_key     text,
    fingerprint         text        NOT NULL,
    attempts            integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts        integer     NOT NULL CHECK (max_attempts > 0),
    scheduled_at        timestamptz,
    next_attempt_at     timestamptz NOT NULL,
    -- A lease is held while (and only while) a worker delivers the row.
    lease_id            uuid,
    lease_expires_at    timestamptz,
    last_error          text,
    provider_message_id text,
    correlation_id      text,
    trace_parent        text,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    sent_at             timestamptz,
    failed_at           timestamptz,
    cancelled_at        timestamptz,

    CONSTRAINT notifications_lease_matches_status
        CHECK ((status = 'processing') = (lease_id IS NOT NULL AND lease_expires_at IS NOT NULL))
);

-- One notification per client-supplied idempotency key.
CREATE UNIQUE INDEX notifications_idempotency_key_key
    ON notifications (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- The queue: due notifications per channel in priority order. The index is
-- partial, so it only holds waiting rows and stays small however large the
-- table grows.
CREATE INDEX notifications_queue_idx
    ON notifications (channel, priority_rank, next_attempt_at)
    WHERE status IN ('scheduled', 'queued', 'retrying');

-- Lease recovery: rows whose worker died while delivering them.
CREATE INDEX notifications_expired_lease_idx
    ON notifications (lease_expires_at)
    WHERE status = 'processing';

-- Listing newest first with keyset pagination, alone or filtered.
CREATE INDEX notifications_created_idx ON notifications (created_at DESC, id DESC);
CREATE INDEX notifications_status_created_idx ON notifications (status, created_at DESC, id DESC);
CREATE INDEX notifications_channel_created_idx ON notifications (channel, created_at DESC, id DESC);
CREATE INDEX notifications_recipient_created_idx ON notifications (recipient, created_at DESC, id DESC);
CREATE INDEX notifications_batch_idx ON notifications (batch_id, id) WHERE batch_id IS NOT NULL;
