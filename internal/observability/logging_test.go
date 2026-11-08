package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceHandlerAddsSpanIdentity(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(&TraceHandler{Handler: slog.NewJSONHandler(&output, nil)})
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	require.NoError(t, err)
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	logger.InfoContext(trace.ContextWithSpanContext(context.Background(), spanContext), "traced", "key", "value")
	var entry map[string]string
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	require.Equal(t, traceID.String(), entry["trace_id"])
	require.Equal(t, spanID.String(), entry["span_id"])
	require.Equal(t, "value", entry["key"])
}

func TestTraceHandlerWithoutSpan(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(&TraceHandler{Handler: slog.NewJSONHandler(&output, nil)})
	logger.InfoContext(context.Background(), "plain")
	var entry map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	require.NotContains(t, entry, "trace_id")
}
