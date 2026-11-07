package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type storageFunc struct {
	upload func(context.Context, string, string, []byte) error
}

func (s storageFunc) Upload(ctx context.Context, key, contentType string, data []byte) error {
	return s.upload(ctx, key, contentType, data)
}
func (s storageFunc) Download(context.Context, string) (Object, error) { return Object{}, nil }
func (s storageFunc) Delete(context.Context, string) error             { return nil }
func (s storageFunc) EnsureBucket(context.Context) error               { return nil }
func (s storageFunc) Ping(context.Context) error                       { return nil }

func TestTracedStorageRecordsErrors(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	expected := errors.New("s3 unavailable")
	storage := NewTracedStorage(storageFunc{upload: func(context.Context, string, string, []byte) error { return expected }})
	err := storage.Upload(context.Background(), "avatar", "image/png", []byte("data"))
	require.ErrorIs(t, err, expected)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "s3.upload", spans[0].Name())
	require.Equal(t, codes.Error, spans[0].Status().Code)
}
