-- +goose Up
-- expires_at: quando essa mídia deve ser apagada do storage (S3/MinIO) + DB
-- pelo coletor. NULL = guarda pra sempre.
-- +goose StatementBegin
ALTER TABLE media ADD COLUMN IF NOT EXISTS expires_at timestamptz;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS media_expires_idx ON media (expires_at) WHERE expires_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS media_expires_idx;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE media DROP COLUMN IF EXISTS expires_at;
-- +goose StatementEnd
