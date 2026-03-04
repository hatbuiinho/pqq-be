package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"pqq/be/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type UploadObjectInput struct {
	Key          string
	ContentType  string
	CacheControl string
	UserMetadata map[string]string
	Size         int64
	Body         io.Reader
}

type StoredObject struct {
	Bucket      string
	Key         string
	ETag        string
	ContentType string
	Size        int64
}

type Service interface {
	EnsureBucket(ctx context.Context) error
	UploadObject(ctx context.Context, input UploadObjectInput) (*StoredObject, error)
	PresignUploadURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error)
	PresignDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error)
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, key string) error
	PublicObjectURL(key string) string
	Bucket() string
}

type MinIOService struct {
	client    *minio.Client
	bucket    string
	region    string
	publicURL string
}

func NewMinIOService(cfg config.StorageConfig) (*MinIOService, error) {
	if !cfg.Enabled {
		return nil, errors.New("minio storage is disabled")
	}
	if cfg.Endpoint == "" || cfg.AccessKey == "" || cfg.SecretKey == "" || cfg.Bucket == "" {
		return nil, errors.New("minio storage config is incomplete")
	}

	endpoint, secure, err := normalizeEndpoint(cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		return nil, err
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &MinIOService{
		client:    client,
		bucket:    cfg.Bucket,
		region:    cfg.Region,
		publicURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}, nil
}

func normalizeEndpoint(raw string, useSSL bool) (string, bool, error) {
	if raw == "" {
		return "", useSSL, errors.New("minio endpoint is required")
	}

	if !strings.Contains(raw, "://") {
		return strings.TrimSpace(raw), useSSL, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", useSSL, fmt.Errorf("parse minio endpoint: %w", err)
	}
	if parsed.Host == "" {
		return "", useSSL, errors.New("minio endpoint host is required")
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http":
		useSSL = false
	case "https":
		useSSL = true
	}

	return parsed.Host, useSSL, nil
}

func (s *MinIOService) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check minio bucket: %w", err)
	}
	if exists {
		return nil
	}

	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{
		Region: s.region,
	}); err != nil {
		return fmt.Errorf("create minio bucket: %w", err)
	}

	return nil
}

func (s *MinIOService) UploadObject(ctx context.Context, input UploadObjectInput) (*StoredObject, error) {
	if input.Key == "" {
		return nil, errors.New("storage key is required")
	}
	if input.Body == nil {
		return nil, errors.New("upload body is required")
	}

	info, err := s.client.PutObject(ctx, s.bucket, input.Key, input.Body, input.Size, minio.PutObjectOptions{
		ContentType:  input.ContentType,
		CacheControl: input.CacheControl,
		UserMetadata: input.UserMetadata,
	})
	if err != nil {
		return nil, fmt.Errorf("upload object to minio: %w", err)
	}

	return &StoredObject{
		Bucket:      s.bucket,
		Key:         input.Key,
		ETag:        strings.Trim(info.ETag, `"`),
		ContentType: input.ContentType,
		Size:        info.Size,
	}, nil
}

func (s *MinIOService) PresignUploadURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	if key == "" {
		return "", errors.New("storage key is required")
	}
	_ = contentType

	presignedURL, err := s.client.PresignedPutObject(ctx, s.bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("presign upload url: %w", err)
	}

	return presignedURL.String(), nil
}

func (s *MinIOService) PresignDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if key == "" {
		return "", errors.New("storage key is required")
	}

	presignedURL, err := s.client.PresignedGetObject(ctx, s.bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presign download url: %w", err)
	}

	return presignedURL.String(), nil
}

func (s *MinIOService) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, errors.New("storage key is required")
	}

	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object from minio: %w", err)
	}

	if _, err := object.Stat(); err != nil {
		_ = object.Close()
		return nil, fmt.Errorf("stat object from minio: %w", err)
	}

	return object, nil
}

func (s *MinIOService) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return errors.New("storage key is required")
	}

	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object from minio: %w", err)
	}

	return nil
}

func (s *MinIOService) PublicObjectURL(key string) string {
	if s.publicURL == "" || key == "" {
		return ""
	}

	return s.publicURL + "/" + path.Clean(strings.TrimPrefix(key, "/"))
}

func (s *MinIOService) Bucket() string {
	return s.bucket
}
