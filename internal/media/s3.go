package media

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config configura um backend S3-compativel (AWS S3, MinIO, Cloudflare R2).
type S3Config struct {
	Endpoint      string // ex.: "s3.amazonaws.com" ou "minio:9000"
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	PathStyle     bool   // true para MinIO/R2
	PublicBaseURL string // se setado, URL() devolve link publico direto em vez de presigned
}

type s3Store struct {
	cli    *minio.Client
	cfg    S3Config
	pubURL *url.URL
}

// NewS3 conecta no bucket e garante que ele existe.
func NewS3(ctx context.Context, cfg S3Config) (Store, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("media s3: endpoint e bucket sao obrigatorios")
	}
	// tolera S3_ENDPOINT com esquema/barra ("https://host/" -> "host" + UseSSL).
	if strings.HasPrefix(cfg.Endpoint, "http://") {
		cfg.Endpoint, cfg.UseSSL = strings.TrimPrefix(cfg.Endpoint, "http://"), false
	} else if strings.HasPrefix(cfg.Endpoint, "https://") {
		cfg.Endpoint, cfg.UseSSL = strings.TrimPrefix(cfg.Endpoint, "https://"), true
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")

	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.UseSSL,
		Region:       cfg.Region,
		BucketLookup: bucketLookup(cfg.PathStyle),
	})
	if err != nil {
		return nil, fmt.Errorf("media s3: %w", err)
	}

	exists, err := cli.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		er := minio.ToErrorResponse(err)
		hint := ""
		if er.Code == "" || er.Code == "BadRequest" {
			hint = " — a resposta nao parece de uma API S3; confira se S3_ENDPOINT aponta para a porta da API (9000), nao para o painel (9001)"
		} else if er.Code == "AccessDenied" || er.Code == "SignatureDoesNotMatch" {
			hint = " — credenciais sem permissao; crie o bucket na mao no painel ou use uma chave com acesso total"
		}
		return nil, fmt.Errorf("media s3: BucketExists (endpoint=%s ssl=%v code=%q status=%d): %w%s",
			cfg.Endpoint, cfg.UseSSL, er.Code, er.StatusCode, err, hint)
	}
	if !exists {
		if err := cli.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("media s3: MakeBucket (endpoint=%s): %w — se for permissao, crie o bucket %q no painel do MinIO",
				cfg.Endpoint, err, cfg.Bucket)
		}
	}

	st := &s3Store{cli: cli, cfg: cfg}
	if cfg.PublicBaseURL != "" {
		u, err := url.Parse(cfg.PublicBaseURL)
		if err != nil {
			return nil, fmt.Errorf("media s3: PublicBaseURL invalida: %w", err)
		}
		st.pubURL = u
	}
	return st, nil
}

func bucketLookup(pathStyle bool) minio.BucketLookupType {
	if pathStyle {
		return minio.BucketLookupPath
	}
	return minio.BucketLookupAuto
}

func (s *s3Store) Enabled() bool { return true }

func (s *s3Store) Put(ctx context.Context, key, mimetype string, r io.Reader, size int64) (Object, error) {
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	if size <= 0 {
		size = -1 // deixa o SDK fazer streaming em partes
	}
	info, err := s.cli.PutObject(ctx, s.cfg.Bucket, key, r, size, minio.PutObjectOptions{ContentType: mimetype})
	if err != nil {
		return Object{}, fmt.Errorf("media s3 put: %w", err)
	}
	return Object{Ref: key, Mimetype: mimetype, Size: info.Size}, nil
}

func (s *s3Store) Get(ctx context.Context, ref string) (io.ReadCloser, string, error) {
	obj, err := s.cli.GetObject(ctx, s.cfg.Bucket, ref, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("media s3 get: %w", err)
	}
	stat, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, "", fmt.Errorf("media s3 stat: %w", err)
	}
	return obj, stat.ContentType, nil
}

func (s *s3Store) PresignedURL(ctx context.Context, ref string, ttl time.Duration) (string, error) {
	if s.pubURL != nil {
		u := *s.pubURL
		u.Path = strings.TrimRight(u.Path, "/") + "/" + ref
		return u.String(), nil
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	u, err := s.cli.PresignedGetObject(ctx, s.cfg.Bucket, ref, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("media s3 presign: %w", err)
	}
	return u.String(), nil
}
