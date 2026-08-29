// Package media abstrai o armazenamento de binarios de midia (imagens,
// audios, documentos recebidos/enviados). Backend padrao: desligado. Com
// MEDIA_BACKEND=s3 usa um bucket S3-compativel (AWS S3, MinIO, R2, ...).
package media

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrDisabled e devolvido quando nenhum backend de midia esta configurado.
var ErrDisabled = errors.New("armazenamento de midia desligado")

// Object descreve um binario guardado.
type Object struct {
	Ref      string // identificador opaco para recuperar depois (ex.: chave no bucket)
	Mimetype string
	Size     int64
}

// Store guarda e recupera binarios de midia.
type Store interface {
	Enabled() bool
	Put(ctx context.Context, key, mimetype string, r io.Reader, size int64) (Object, error)
	Get(ctx context.Context, ref string) (io.ReadCloser, string, error)
	// PresignedURL devolve uma URL temporaria de download direto, se o
	// backend suportar; senao ErrDisabled.
	PresignedURL(ctx context.Context, ref string, ttl time.Duration) (string, error)
}

// Disabled e o backend nulo (padrao).
type Disabled struct{}

func (Disabled) Enabled() bool { return false }
func (Disabled) Put(context.Context, string, string, io.Reader, int64) (Object, error) {
	return Object{}, ErrDisabled
}
func (Disabled) Get(context.Context, string) (io.ReadCloser, string, error) {
	return nil, "", ErrDisabled
}
func (Disabled) PresignedURL(context.Context, string, time.Duration) (string, error) {
	return "", ErrDisabled
}
