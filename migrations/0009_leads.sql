-- +goose Up
-- Um "lead" por (sessão, contato): métricas de conversa prontas pra alimentar
-- um CRM (últimas mensagens, tempo sem resposta, contagens, tempo médio de
-- resposta) + atribuição da origem (anúncio Click-to-WhatsApp, UTMs, click ids).
-- +goose StatementBegin
CREATE TABLE leads (
    session               text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    chat_id               text NOT NULL,
    phone                 text NOT NULL DEFAULT '',
    push_name             text NOT NULL DEFAULT '',
    first_contact_at      timestamptz NOT NULL,
    last_inbound_at       timestamptz,
    last_outbound_at      timestamptz,
    last_read_by_them_at  timestamptz,
    inbound_count         int NOT NULL DEFAULT 0,
    outbound_count        int NOT NULL DEFAULT 0,
    response_count        int NOT NULL DEFAULT 0,
    response_seconds_total bigint NOT NULL DEFAULT 0,
    status                text NOT NULL DEFAULT 'new',   -- new | waiting_us | waiting_them | closed
    waiting_since         timestamptz,                   -- se waiting_us: 1º inbound sem resposta
    stage                 text NOT NULL DEFAULT '',      -- pipeline do CRM (livre)
    owner                 text NOT NULL DEFAULT '',
    tags                  text[] NOT NULL DEFAULT '{}',
    notes                 text NOT NULL DEFAULT '',
    source                jsonb NOT NULL DEFAULT '{}',   -- {adReferral, utm, clickIds, params, firstMessage}
    last_message          text NOT NULL DEFAULT '',
    last_message_from_me  boolean NOT NULL DEFAULT false,
    stale_notified        boolean NOT NULL DEFAULT false,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (session, chat_id)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX leads_session_updated_idx ON leads (session, updated_at DESC);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX leads_session_status_idx ON leads (session, status);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX leads_waiting_idx ON leads (session, waiting_since) WHERE status = 'waiting_us';
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX leads_stage_idx ON leads (session, stage) WHERE stage <> '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS leads;
-- +goose StatementEnd
