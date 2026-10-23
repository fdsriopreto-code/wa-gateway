package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsPermanent(t *testing.T) {
	cases := map[string]bool{
		"23503": true,  // FK (sessão apagada)
		"22021": true,  // UTF-8 inválido
		"40001": false, // serialization failure: vale repetir
		"57P01": false, // admin shutdown
		"08006": false, // conexão caiu
	}
	for code, want := range cases {
		err := fmt.Errorf("embrulhado: %w", &pgconn.PgError{Code: code})
		if got := IsPermanent(err); got != want {
			t.Errorf("%s: IsPermanent=%v, quero %v", code, got, want)
		}
	}
	if IsPermanent(errors.New("timeout")) || IsPermanent(nil) {
		t.Error("erro que não é do Postgres não é permanente")
	}
}
