package storage

import (
	"context"
	"io"
)

type Object struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

type Storage interface {
	Upload(ctx context.Context, key, contentType string, data []byte) error
	Download(ctx context.Context, key string) (Object, error)
	Delete(ctx context.Context, key string) error
	EnsureBucket(ctx context.Context) error
	Ping(ctx context.Context) error
}
