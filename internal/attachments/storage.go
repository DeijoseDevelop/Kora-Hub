// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
// Package attachments implementa el backend de bytes para adjuntos
// (sección 10): interfaz Storage con dos implementaciones — local
// (filesystem bajo data/attachments) y S3-compatible (minio-go).
// El índice SQLite solo guarda metadatos + storage_key.
package attachments

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Storage es el backend de bytes de adjuntos.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, mimeType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// S3Config agrupa los env vars del backend S3.
type S3Config struct {
	Endpoint  string // HUB_S3_ENDPOINT
	Bucket    string // HUB_S3_BUCKET
	AccessKey string // HUB_S3_ACCESS_KEY
	SecretKey string // HUB_S3_SECRET_KEY
	Region    string // HUB_S3_REGION (opcional)
	UseSSL    bool   // HUB_S3_SSL (default true)
}

// NewStorage construye el backend según config: "local" o "s3".
func NewStorage(kind, dataDir string, s3cfg *S3Config) (Storage, error) {
	switch kind {
	case "s3":
		if s3cfg == nil || s3cfg.Endpoint == "" || s3cfg.Bucket == "" {
			return nil, fmt.Errorf("HUB_STORAGE=s3 requiere HUB_S3_ENDPOINT y HUB_S3_BUCKET")
		}
		return newS3(s3cfg)
	default:
		return NewLocal(filepath.Join(dataDir, "attachments")), nil
	}
}

// ------------------------------ Local --------------------------------

// Local guarda los bytes bajo <dataDir>/attachments/<key>.
type Local struct {
	root string
}

func NewLocal(root string) *Local { return &Local{root: root} }

func (l *Local) path(key string) string {
	clean := filepath.Clean(key)
	full := filepath.Join(l.root, clean)
	if rel, err := filepath.Rel(l.root, full); err != nil || strings.HasPrefix(rel, "..") {
		return filepath.Join(l.root, "invalid")
	}
	return full
}

func (l *Local) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	full := l.path(key)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (l *Local) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(l.path(key))
}

func (l *Local) Delete(_ context.Context, key string) error {
	err := os.Remove(l.path(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ------------------------------- S3 ----------------------------------

// S3 usa minio-go contra cualquier servicio S3-compatible (AWS, MinIO,
// R2, Spaces). Los objetos van bajo el bucket/prefijo de la instancia.
type S3 struct {
	client *minio.Client
	bucket string
}

func newS3(cfg *S3Config) (*S3, error) {
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}
	return &S3{client: cli, bucket: cfg.Bucket}, nil
}

// EnsureBucket crea el bucket si no existe (idempotente, se llama al
// arrancar con storage=s3).
func (s *S3) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: ""})
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, mimeType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size,
		minio.PutObjectOptions{ContentType: mimeType})
	return err
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// ------------------------------ MIME ---------------------------------

// MIME permitidos (whitelist, sección 11): imágenes, audio, video,
// documentos comunes y datos. Nada ejecutable.
var allowedMIMEPrefixes = []string{
	"image/", "audio/", "video/", "text/",
}

var allowedMIMETypes = map[string]bool{
	"application/pdf":    true,
	"application/json":   true,
	"application/zip":    true,
	"application/x-tar":  true,
	"application/gzip":   true,
	"application/x-yaml": true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	// octet-stream NO entra: es lo que devuelve el sniffer para .exe/.dll
}

// AllowedMIME valida el MIME declarado/sniffado contra la whitelist.
func AllowedMIME(mimeType string) bool {
	base, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		base = mimeType
	}
	base = strings.ToLower(strings.TrimSpace(base))
	if allowedMIMETypes[base] {
		return true
	}
	for _, p := range allowedMIMEPrefixes {
		if strings.HasPrefix(base, p) {
			return true
		}
	}
	return false
}

// SniffMIME detecta el tipo real de los primeros bytes (el navegador
// puede mentir el Content-Type del multipart).
func SniffMIME(head []byte) string {
	return http.DetectContentType(head)
}
