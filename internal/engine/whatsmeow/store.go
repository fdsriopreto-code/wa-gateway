package whatsmeow

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// Um único sqlstore.Container (= um pool de conexões) por processo, dividido
// por todas as sessões. Antes cada sessão abria o seu (sql.Open sem limite +
// migração do whatsmeow) e nunca fechava no Stop: com centenas de sessões o
// Postgres estourava max_connections, e cada restart vazava um pool.
var (
	sharedMu   sync.Mutex
	containers = map[string]*sqlstore.Container{}
)

// storeMaxConns: WA_STORE_MAX_CONNS (padrão 30) — teto de conexões do pool
// do whatsmeow neste nó, somado ao DATABASE_MAX_CONNS do resto do app.
func storeMaxConns() int {
	if n, err := strconv.Atoi(os.Getenv("WA_STORE_MAX_CONNS")); err == nil && n > 0 {
		return n
	}
	return 30
}

func sharedContainer(ctx context.Context, dsn string) (*sqlstore.Container, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if c, ok := containers[dsn]; ok {
		return c, nil
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	n := storeMaxConns()
	db.SetMaxOpenConns(n)
	db.SetMaxIdleConns(n / 2)
	db.SetConnMaxIdleTime(5 * time.Minute)
	c := sqlstore.NewWithDB(db, "pgx", waLog.Stdout("wa-sqlstore", "WARN", false))
	if err := c.Upgrade(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	containers[dsn] = c
	return c, nil
}
