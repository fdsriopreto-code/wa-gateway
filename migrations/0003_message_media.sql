-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS push_name           text,
    ADD COLUMN IF NOT EXISTS media_mimetype       text,
    ADD COLUMN IF NOT EXISTS media_filename       text,
    ADD COLUMN IF NOT EXISTS media_direct_path    text,
    ADD COLUMN IF NOT EXISTS media_key            bytea,
    ADD COLUMN IF NOT EXISTS media_file_sha256    bytea,
    ADD COLUMN IF NOT EXISTS media_file_enc_sha256 bytea,
    ADD COLUMN IF NOT EXISTS media_file_length    bigint;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS messages_session_ts_idx ON messages (session, timestamp DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE messages
    DROP COLUMN IF EXISTS push_name,
    DROP COLUMN IF EXISTS media_mimetype,
    DROP COLUMN IF EXISTS media_filename,
    DROP COLUMN IF EXISTS media_direct_path,
    DROP COLUMN IF EXISTS media_key,
    DROP COLUMN IF EXISTS media_file_sha256,
    DROP COLUMN IF EXISTS media_file_enc_sha256,
    DROP COLUMN IF EXISTS media_file_length;
-- +goose StatementEnd
