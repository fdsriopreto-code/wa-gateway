// Package secret cifra valores sensíveis em repouso (hoje: HMAC secret de
// webhook em sessions.config e webhook_deliveries.payload). AES-256-GCM com
// chave de env (SECRET_KEY = 64 hex chars). Sem chave, é passthrough — o
// campo continua em texto puro e nada quebra.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const prefix = "enc:v1:"

type Box struct {
	aead cipher.AEAD // nil = desligado (passthrough)
}

// New monta o Box. keyHex vazio => passthrough. keyHex inválido => erro
// (falha explícita no boot é melhor que cifrar com chave errada).
func New(keyHex string) (*Box, error) {
	keyHex = strings.TrimSpace(keyHex)
	if keyHex == "" {
		return &Box{}, nil
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("SECRET_KEY deve ser 64 caracteres hex (32 bytes AES-256)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Enabled diz se há chave configurada.
func (b *Box) Enabled() bool { return b != nil && b.aead != nil }

// Seal cifra s. No-op se o Box está desligado, s vazio, ou s já cifrado.
func (b *Box) Seal(s string) string {
	if !b.Enabled() || s == "" || strings.HasPrefix(s, prefix) {
		return s
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return s
	}
	ct := b.aead.Seal(nonce, nonce, []byte(s), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(ct)
}

// Open decifra s. Devolve s como está se: não tem o prefixo (texto puro
// legado), o Box está desligado, ou a decifragem falha.
func (b *Box) Open(s string) string {
	if !strings.HasPrefix(s, prefix) || !b.Enabled() {
		return s
	}
	raw, err := base64.RawStdEncoding.DecodeString(s[len(prefix):])
	if err != nil || len(raw) < b.aead.NonceSize() {
		return s
	}
	ns := b.aead.NonceSize()
	pt, err := b.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return s
	}
	return string(pt)
}
