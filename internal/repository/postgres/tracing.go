package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type TracedPool struct {
	Pool
}

func NewTracedPool(pool Pool) *TracedPool {
	return &TracedPool{Pool: pool}
}

func (p *TracedPool) start(ctx context.Context, operation, query string) (context.Context, trace.Span) {
	attributes := []attribute.KeyValue{
		attribute.String("db.system.name", "postgresql"),
		attribute.String("db.operation.name", operation),
	}
	if query != "" {
		attributes = append(attributes, attribute.String("db.query.text", query))
	}
	return otel.Tracer("gophprofile/postgres").Start(ctx, "postgres."+strings.ToLower(operation),
		trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attributes...))
}

func endDBSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

func (p *TracedPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	ctx, span := p.start(ctx, "EXEC", sql)
	result, err := p.Pool.Exec(ctx, sql, args...)
	endDBSpan(span, err)
	return result, err
}

func (p *TracedPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	ctx, span := p.start(ctx, "QUERY", sql)
	rows, err := p.Pool.Query(ctx, sql, args...)
	endDBSpan(span, err)
	return rows, err
}

type tracedRow struct {
	row  pgx.Row
	span trace.Span
}

func (r *tracedRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	endDBSpan(r.span, err)
	return err
}

func (p *TracedPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	ctx, span := p.start(ctx, "QUERY_ROW", sql)
	return &tracedRow{row: p.Pool.QueryRow(ctx, sql, args...), span: span}
}

func (p *TracedPool) Begin(ctx context.Context) (pgx.Tx, error) {
	ctx, span := p.start(ctx, "BEGIN", "")
	tx, err := p.Pool.Begin(ctx)
	endDBSpan(span, err)
	return tx, err
}

func (p *TracedPool) Ping(ctx context.Context) error {
	ctx, span := p.start(ctx, "PING", "")
	err := p.Pool.Ping(ctx)
	endDBSpan(span, err)
	return err
}
