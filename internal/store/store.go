// Package store e o acesso ao PostgreSQL: pool pgx + migracoes goose.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // driver database/sql "pgx", usado pelo goose e pelo whatsmeow
	"github.com/pressly/goose/v3"

	"wa-gateway/migrations"
)

type Store struct {
	Pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

// Migrate aplica as migracoes da aplicacao (tabelas wa_*). As tabelas
// whatsmeow_* sao criadas pelo proprio sqlstore da engine.
func Migrate(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open sql: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	goose.SetTableName("wa_goose_migrations")
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
