-- +goose Up
-- +goose StatementBegin
CREATE TABLE sessions (
    name       text PRIMARY KEY,
    engine     text NOT NULL DEFAULT 'whatsmeow',
    status     text NOT NULL DEFAULT 'STOPPED',
    jid        text,
    push_name  text,
    config     jsonb NOT NULL DEFAULT '{}',
    me         jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE api_keys (
    id           text PRIMARY KEY,
    hash         text NOT NULL,
    scopes       text[] NOT NULL DEFAULT '{}',
    label        text,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE webhook_deliveries (
    id            text PRIMARY KEY,
    session       text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    url           text NOT NULL,
    event         text NOT NULL,
    status        text NOT NULL DEFAULT 'pending',
    attempts      int  NOT NULL DEFAULT 0,
    last_error    text,
    response_code int,
    created_at    timestamptz NOT NULL DEFAULT now(),
    delivered_at  timestamptz
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX webhook_deliveries_session_idx ON webhook_deliveries (session, created_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE messages (
    session     text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    id          text NOT NULL,
    chat_jid    text NOT NULL,
    sender_jid  text,
    from_me     boolean NOT NULL,
    type        text NOT NULL,
    timestamp   timestamptz NOT NULL,
    body        text,
    payload     jsonb NOT NULL,
    ack         smallint NOT NULL DEFAULT 0,
    media_id    text,
    PRIMARY KEY (session, id)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX messages_chat_idx ON messages (session, chat_jid, timestamp DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE chats (
    session    text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    jid        text NOT NULL,
    name       text,
    is_group   boolean NOT NULL DEFAULT false,
    unread     int NOT NULL DEFAULT 0,
    archived   boolean NOT NULL DEFAULT false,
    labels     text[] NOT NULL DEFAULT '{}',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (session, jid)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE contacts (
    session   text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    jid       text NOT NULL,
    push_name text,
    full_name text,
    pic_url   text,
    PRIMARY KEY (session, jid)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE media (
    id         text PRIMARY KEY,
    session    text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    mimetype   text NOT NULL,
    size       bigint,
    backend    text NOT NULL,
    ref        text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE integration_sessions (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session    text NOT NULL REFERENCES sessions(name) ON DELETE CASCADE,
    remote_jid text NOT NULL,
    bot_id     text NOT NULL,
    status     text NOT NULL DEFAULT 'opened',
    await_user boolean NOT NULL DEFAULT false,
    context    jsonb,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session, remote_jid, bot_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS integration_sessions;
DROP TABLE IF EXISTS media;
DROP TABLE IF EXISTS contacts;
DROP TABLE IF EXISTS chats;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS sessions;
-- +goose StatementEnd
