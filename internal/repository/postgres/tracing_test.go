package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracedPool(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	expected := errors.New("database unavailable")
	mock.ExpectExec("UPDATE avatars").WillReturnError(expected)
	pool := NewTracedPool(mock)
	_, err = pool.Exec(context.Background(), "UPDATE avatars SET processing_status = 'failed'")
	require.ErrorIs(t, err, expected)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "postgres.exec", spans[0].Name())
	require.Equal(t, codes.Error, spans[0].Status().Code)
}
