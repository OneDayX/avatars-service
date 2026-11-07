package storage

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type TracedStorage struct {
	storage Storage
}

func NewTracedStorage(storage Storage) *TracedStorage {
	return &TracedStorage{storage: storage}
}

func (s *TracedStorage) start(ctx context.Context, operation, key string) (context.Context, trace.Span) {
	attributes := []attribute.KeyValue{attribute.String("s3.operation", operation)}
	if key != "" {
		attributes = append(attributes, attribute.String("s3.object.key", key))
	}
	return otel.Tracer("gophprofile/s3").Start(ctx, "s3."+operation,
		trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attributes...))
}

func endS3Span(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

func (s *TracedStorage) Upload(ctx context.Context, key, contentType string, data []byte) error {
	ctx, span := s.start(ctx, "upload", key)
	span.SetAttributes(attribute.Int("s3.object.size_bytes", len(data)))
	err := s.storage.Upload(ctx, key, contentType, data)
	endS3Span(span, err)
	return err
}

func (s *TracedStorage) Download(ctx context.Context, key string) (Object, error) {
	ctx, span := s.start(ctx, "download", key)
	object, err := s.storage.Download(ctx, key)
	endS3Span(span, err)
	return object, err
}

func (s *TracedStorage) Delete(ctx context.Context, key string) error {
	ctx, span := s.start(ctx, "delete", key)
	err := s.storage.Delete(ctx, key)
	endS3Span(span, err)
	return err
}

func (s *TracedStorage) EnsureBucket(ctx context.Context) error {
	ctx, span := s.start(ctx, "ensure_bucket", "")
	err := s.storage.EnsureBucket(ctx)
	endS3Span(span, err)
	return err
}

func (s *TracedStorage) Ping(ctx context.Context) error {
	ctx, span := s.start(ctx, "ping", "")
	err := s.storage.Ping(ctx)
	endS3Span(span, err)
	return err
}
