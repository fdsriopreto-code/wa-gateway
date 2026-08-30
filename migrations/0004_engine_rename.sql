-- +goose Up
-- O identificador publico da engine deixou de expor o nome da lib ("whatsmeow")
-- e passou a ser "wa-gateway". Atualiza os registros existentes e o default.
-- +goose StatementBegin
UPDATE sessions SET engine = 'wa-gateway' WHERE engine = 'whatsmeow';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE sessions ALTER COLUMN engine SET DEFAULT 'wa-gateway';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sessions ALTER COLUMN engine SET DEFAULT 'whatsmeow';
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE sessions SET engine = 'whatsmeow' WHERE engine = 'wa-gateway';
-- +goose StatementEnd
