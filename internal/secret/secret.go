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
	aead cipher.AEAD   // chave primária (cifra + 1ª tentativa de decifra); nil = passthrough
	old  []cipher.AEAD // chaves antigas: só decifram (rotação sem downtime)
}

func aeadFrom(keyHex string) (cipher.AEAD, error) {
	key, err := hex.DecodeString(strings.TrimSpace(keyHex))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("chave deve ser 64 caracteres hex (32 bytes AES-256)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// New monta o Box. primaryHex vazio => passthrough. oldHex são chaves só de
// leitura (rotação): valores cifrados com elas ainda abrem, novos vão com a
// primária. Chave inválida => erro (falha explícita no boot).
func New(primaryHex string, oldHex ...string) (*Box, error) {
	b := &Box{}
	if strings.TrimSpace(primaryHex) != "" {
		a, err := aeadFrom(primaryHex)
		if err != nil {
			return nil, fmt.Errorf("SECRET_KEY: %w", err)
		}
		b.aead = a
	}
	for _, h := range oldHex {
		if strings.TrimSpace(h) == "" {
			continue
		}
		a, err := aeadFrom(h)
		if err != nil {
			return nil, fmt.Errorf("SECRET_KEY_OLD: %w", err)
		}
		b.old = append(b.old, a)
	}
	return b, nil
}

// Enabled diz se há chave primária configurada.
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

// Open decifra s. Tenta a chave primária e depois as antigas. Devolve s como
// está se: não tem o prefixo (texto puro legado), o Box está desligado, ou
// nenhuma chave abre.
func (b *Box) Open(s string) string {
	if !strings.HasPrefix(s, prefix) || !b.Enabled() {
		return s
	}
	raw, err := base64.RawStdEncoding.DecodeString(s[len(prefix):])
	if err != nil {
		return s
	}
	for _, a := range append([]cipher.AEAD{b.aead}, b.old...) {
		ns := a.NonceSize()
		if len(raw) < ns {
			continue
		}
		if pt, err := a.Open(nil, raw[:ns], raw[ns:], nil); err == nil {
			return string(pt)
		}
	}
	return s
}
