package minio

import (
	"bytes"
	"context"
	"fmt"
	"time"

	base "github.com/example/gophprofile/internal/storage"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage struct {
	client *miniosdk.Client
	bucket string
}

func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Storage, error) {
	client, err := miniosdk.New(endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &Storage{client: client, bucket: bucket}, nil
}

func (s *Storage) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, miniosdk.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func (s *Storage) Upload(ctx context.Context, key, contentType string, data []byte) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), miniosdk.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("upload object %s: %w", key, err)
	}
	return nil
}

func (s *Storage) Download(ctx context.Context, key string) (base.Object, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, miniosdk.GetObjectOptions{})
	if err != nil {
		return base.Object{}, fmt.Errorf("download object %s: %w", key, err)
	}
	info, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return base.Object{}, fmt.Errorf("stat object %s: %w", key, err)
	}
	return base.Object{Body: object, ContentType: info.ContentType, Size: info.Size}, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, miniosdk.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}

func (s *Storage) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", s.bucket)
	}
	return nil
}

var _ base.Storage = (*Storage)(nil)
