package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsPermanent diz se o erro do Postgres não se resolve repetindo: classe 22
// (dado inválido, ex.: UTF-8 quebrado) ou 23 (constraint, ex.: FK de uma
// sessão já apagada). Consumidores do stream usam pra descartar o evento em
// vez de reprocessar pra sempre.
func IsPermanent(err error) bool {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || len(pg.Code) < 2 {
		return false
	}
	switch pg.Code[:2] {
	case "22", "23":
		return true
	}
	return false
}
