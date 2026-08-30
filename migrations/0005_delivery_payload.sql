-- +goose Up
-- Guarda o payload completo da entrega (envelope + secret + headers) pra
-- permitir reenvio manual via POST /api/deliveries/{id}/retry.
-- +goose StatementBegin
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS payload jsonb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS payload;
-- +goose StatementEnd
