-- Reusable message bodies with {{variable}} placeholders.
CREATE TABLE templates (
    id         uuid        PRIMARY KEY,
    name       text        NOT NULL,
    channel    text        NOT NULL CHECK (channel IN ('sms', 'email', 'push')),
    body       text        NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,

    CONSTRAINT templates_name_key UNIQUE (name)
);

-- Notifications remember the template they were rendered from. Content is
-- rendered at creation time, so deleting a template only clears the link.
ALTER TABLE notifications
    ADD COLUMN template_id uuid REFERENCES templates (id) ON DELETE SET NULL;
