-- +goose Up
-- Campanhas: envio em massa pausado pelo pacing da fila de saída. Um alvo =
-- um job na outbox_jobs (id = "camp:<campaignId>:<n>"), então o progresso é
-- um agregado sobre campaign_targets, alimentado pelo mesmo worker.
-- +goose StatementBegin
CREATE TABLE campaigns (
    id         text PRIMARY KEY,
    session    text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    name       text NOT NULL DEFAULT '',
    status     text NOT NULL DEFAULT 'running', -- running | done | stopped
    kind       text NOT NULL,                   -- outbox.Kind (text/image/…)
    args       jsonb NOT NULL DEFAULT '{}',     -- Args template (sem chatId)
    pace       jsonb NOT NULL DEFAULT '{}',     -- {minIntervalMs, jitterMs}
    total      int  NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX campaigns_running_idx ON campaigns (created_at DESC) WHERE status = 'running';
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE campaign_targets (
    campaign_id text NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    n           int  NOT NULL,
    chat_id     text NOT NULL,
    status      text NOT NULL DEFAULT 'pending', -- pending | queued | sent | failed
    message_id  text,
    error       text,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, n)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX campaign_targets_pending_idx ON campaign_targets (campaign_id, n) WHERE status = 'pending';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS campaign_targets;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS campaigns;
-- +goose StatementEnd
