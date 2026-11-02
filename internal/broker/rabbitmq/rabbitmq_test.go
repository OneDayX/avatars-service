package rabbitmq

import (
	"testing"

	"github.com/example/gophprofile/internal/broker"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestDecodeUploadDelivery(t *testing.T) {
	delivery, err := decodeDelivery(amqp.Delivery{RoutingKey: string(broker.EventUpload), Body: []byte(`{"avatar_id":"id","user_id":"user","s3_key":"key"}`)})
	require.NoError(t, err)
	require.Equal(t, "id", delivery.Event.Upload.AvatarID)
	require.NotNil(t, delivery.Ack)
}

func TestDecodeDeleteDelivery(t *testing.T) {
	delivery, err := decodeDelivery(amqp.Delivery{RoutingKey: string(broker.EventDelete), Body: []byte(`{"avatar_id":"id","s3_keys":["one","two"]}`)})
	require.NoError(t, err)
	require.Equal(t, []string{"one", "two"}, delivery.Event.Delete.S3Keys)
}

func TestDecodeRejectsInvalidDelivery(t *testing.T) {
	_, err := decodeDelivery(amqp.Delivery{RoutingKey: "unknown", Body: []byte(`{}`)})
	require.Error(t, err)
	_, err = decodeDelivery(amqp.Delivery{RoutingKey: string(broker.EventUpload), Body: []byte(`bad`)})
	require.Error(t, err)
}
