package events

import (
	"strings"

	"github.com/oklog/ulid/v2"
)

// NewID gera um ULID minusculo (mesmo formato do id de envelope do WAHA).
func NewID() string {
	return strings.ToLower(ulid.Make().String())
}
