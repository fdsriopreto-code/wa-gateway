// Package auth: autenticacao por API key (chave-mestra de env + tabela api_keys
// com hash Argon2id) e um middleware chi.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/argon2"
)

type ctxKey int

const principalKey ctxKey = 0

// Principal e a identidade autenticada da requisicao.
type Principal struct {
	KeyID  string
	Scopes []string
}

func (p Principal) Can(scope string) bool {
	for _, s := range p.Scopes {
		if s == "*" || s == scope {
			return true
		}
	}
	return false
}

// CanSession diz se o principal pode operar a sessao `name`. Aceita "*",
// "session:*" (qualquer sessao) ou "session:<name>" (exata).
func (p Principal) CanSession(name string) bool {
	for _, s := range p.Scopes {
		if s == "*" || s == "session:*" || s == "session:"+name {
			return true
		}
	}
	return false
}

// SessionScoped diz se o principal NAO tem acesso irrestrito a sessoes
// (tem so "session:<name>", nunca "*"/"session:*"). Usado para filtrar
// listagens e barrar criacao de sessao nova.
func (p Principal) SessionScoped() bool {
	restricted := false
	for _, s := range p.Scopes {
		switch {
		case s == "*" || s == "session:*":
			return false
		case len(s) > 8 && s[:8] == "session:":
			restricted = true
		}
	}
	return restricted
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// Lookup resolve uma chave crua para os escopos dela; ErrUnknownKey se nao existir.
type Lookup func(ctx context.Context, keyID string) (hash string, scopes []string, err error)

var ErrUnknownKey = errors.New("api key desconhecida")

type Authenticator struct {
	masterKey string
	lookup    Lookup
}

func New(masterKey string, lookup Lookup) *Authenticator {
	return &Authenticator{masterKey: masterKey, lookup: lookup}
}

// tokenFromRequest aceita X-Api-Key, Authorization: Bearer, ou ?api_key= (para /ws).
func tokenFromRequest(r *http.Request) string {
	if v := r.Header.Get("X-Api-Key"); v != "" {
		return v
	}
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return r.URL.Query().Get("api_key")
}

// Middleware exige autenticacao valida em todas as rotas que embrulha.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := tokenFromRequest(r)
		if raw == "" {
			unauthorized(w)
			return
		}

		// chave-mestra
		if a.masterKey != "" && subtle.ConstantTimeCompare([]byte(raw), []byte(a.masterKey)) == 1 {
			r = r.WithContext(context.WithValue(r.Context(), principalKey, Principal{KeyID: "master", Scopes: []string{"*"}}))
			next.ServeHTTP(w, r)
			return
		}

		// formato "<keyID>.<segredo>"
		keyID, secret, ok := strings.Cut(raw, ".")
		if !ok || a.lookup == nil {
			unauthorized(w)
			return
		}
		hash, scopes, err := a.lookup(r.Context(), keyID)
		if err != nil || !VerifyKey(secret, hash) {
			unauthorized(w)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), principalKey, Principal{KeyID: keyID, Scopes: scopes}))
		next.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","message":"API key ausente ou invalida"}`))
}

// ---- Argon2id ----

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

func GenerateKey() (keyID, secret, encodedHash string, err error) {
	idb := make([]byte, 9)
	sb := make([]byte, 24)
	if _, err = rand.Read(idb); err != nil {
		return
	}
	if _, err = rand.Read(sb); err != nil {
		return
	}
	keyID = base64.RawURLEncoding.EncodeToString(idb)
	secret = base64.RawURLEncoding.EncodeToString(sb)
	encodedHash, err = HashKey(secret)
	return
}

func HashKey(secret string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(h)), nil
}

func VerifyKey(secret, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var mem, t, p int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(secret), salt, uint32(t), uint32(mem), uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
