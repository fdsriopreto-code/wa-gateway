-- +goose Up
-- Índice por timestamp (sem sessão) p/ o KPI de mensagens nas últimas 24h.
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS messages_ts_idx ON messages (timestamp DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS messages_ts_idx;
-- +goose StatementEnd
