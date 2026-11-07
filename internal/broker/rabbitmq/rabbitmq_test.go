package rabbitmq

import (
	"context"
	"testing"

	"github.com/example/gophprofile/internal/broker"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestDecodeUploadDelivery(t *testing.T) {
	delivery, err := decodeDelivery(context.Background(), amqp.Delivery{RoutingKey: string(broker.EventUpload), Body: []byte(`{"avatar_id":"id","user_id":"user","s3_key":"key"}`)})
	require.NoError(t, err)
	require.Equal(t, "id", delivery.Event.Upload.AvatarID)
	require.NotNil(t, delivery.Ack)
	require.NotNil(t, delivery.Context)
}

func TestDecodeDeleteDelivery(t *testing.T) {
	delivery, err := decodeDelivery(context.Background(), amqp.Delivery{RoutingKey: string(broker.EventDelete), Body: []byte(`{"avatar_id":"id","s3_keys":["one","two"]}`)})
	require.NoError(t, err)
	require.Equal(t, []string{"one", "two"}, delivery.Event.Delete.S3Keys)
}

func TestDecodeRejectsInvalidDelivery(t *testing.T) {
	_, err := decodeDelivery(context.Background(), amqp.Delivery{RoutingKey: "unknown", Body: []byte(`{}`)})
	require.Error(t, err)
	_, err = decodeDelivery(context.Background(), amqp.Delivery{RoutingKey: string(broker.EventUpload), Body: []byte(`bad`)})
	require.Error(t, err)
}

func TestDeliveryExtractsTraceContext(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	require.NoError(t, err)
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true})
	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(trace.ContextWithSpanContext(context.Background(), spanContext), headersCarrier(headers))
	delivery, err := decodeDelivery(context.Background(), amqp.Delivery{
		RoutingKey: string(broker.EventUpload), Headers: headers,
		Body: []byte(`{"avatar_id":"id","user_id":"user","s3_key":"key"}`),
	})
	require.NoError(t, err)
	require.Equal(t, traceID, trace.SpanContextFromContext(delivery.Context).TraceID())
}
