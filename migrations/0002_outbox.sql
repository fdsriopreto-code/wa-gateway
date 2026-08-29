-- +goose Up
-- +goose StatementBegin
CREATE TABLE outbox_jobs (
    id         text PRIMARY KEY,
    session    text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    kind       text NOT NULL,
    status     text NOT NULL DEFAULT 'scheduled', -- scheduled | sent | failed
    run_at     timestamptz NOT NULL,
    attempts   int  NOT NULL DEFAULT 0,
    message_id text,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at    timestamptz
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX outbox_jobs_session_idx ON outbox_jobs (session, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbox_jobs;
-- +goose StatementEnd
