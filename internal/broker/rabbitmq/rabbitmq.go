package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/gophprofile/internal/broker"
	"github.com/example/gophprofile/internal/domain"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	exchangeName = "avatars.exchange"
	queueName    = "avatars.processing"
)

type Broker struct {
	connection *amqp.Connection
	channel    *amqp.Channel
}

func New(url string) (*Broker, error) {
	connection, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	b := &Broker{connection: connection, channel: channel}
	if err := b.declareTopology(); err != nil {
		_ = b.Close()
		return nil, err
	}
	return b, nil
}

func (b *Broker) declareTopology() error {
	if err := b.channel.ExchangeDeclare(exchangeName, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}
	if _, err := b.channel.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}
	for _, key := range []string{string(broker.EventUpload), string(broker.EventDelete)} {
		if err := b.channel.QueueBind(queueName, key, exchangeName, false, nil); err != nil {
			return fmt.Errorf("bind queue: %w", err)
		}
	}
	return nil
}

func (b *Broker) PublishUpload(ctx context.Context, event domain.AvatarUploadEvent) error {
	return b.publish(ctx, broker.EventUpload, event)
}

func (b *Broker) PublishDelete(ctx context.Context, event domain.AvatarDeleteEvent) error {
	return b.publish(ctx, broker.EventDelete, event)
}

func (b *Broker) publish(ctx context.Context, eventType broker.EventType, event any) error {
	ctx, span := otel.Tracer("gophprofile/rabbitmq").Start(ctx, "rabbitmq.publish "+string(eventType),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(attribute.String("messaging.destination.name", exchangeName), attribute.String("messaging.rabbitmq.destination.routing_key", string(eventType))),
	)
	defer span.End()
	body, err := json.Marshal(event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("marshal event: %w", err)
	}
	messageID := uuid.NewString()
	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(ctx, headersCarrier(headers))
	span.SetAttributes(attribute.String("messaging.message.id", messageID))
	err = b.channel.PublishWithContext(ctx, exchangeName, string(eventType), false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Timestamp:    time.Now().UTC(),
		Headers:      headers,
		Body:         body,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publish %s: %w", eventType, err)
	}
	return nil
}

func (b *Broker) Consume(ctx context.Context) (<-chan broker.Delivery, error) {
	if err := b.channel.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("set consumer prefetch: %w", err)
	}
	messages, err := b.channel.Consume(queueName, "gophprofile-worker", false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("consume queue: %w", err)
	}
	deliveries := make(chan broker.Delivery)
	go func() {
		defer close(deliveries)
		for {
			select {
			case <-ctx.Done():
				return
			case message, ok := <-messages:
				if !ok {
					return
				}
				delivery, err := decodeDelivery(ctx, message)
				if err != nil {
					_ = message.Nack(false, false)
					continue
				}
				select {
				case deliveries <- delivery:
				case <-ctx.Done():
					_ = message.Nack(false, true)
					return
				}
			}
		}
	}()
	return deliveries, nil
}

func decodeDelivery(ctx context.Context, message amqp.Delivery) (broker.Delivery, error) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, headersCarrier(message.Headers))
	event := broker.Event{Type: broker.EventType(message.RoutingKey)}
	switch event.Type {
	case broker.EventUpload:
		if err := json.Unmarshal(message.Body, &event.Upload); err != nil {
			return broker.Delivery{}, err
		}
	case broker.EventDelete:
		if err := json.Unmarshal(message.Body, &event.Delete); err != nil {
			return broker.Delivery{}, err
		}
	default:
		return broker.Delivery{}, fmt.Errorf("unknown routing key %q", message.RoutingKey)
	}
	return broker.Delivery{
		Context: ctx,
		Event:   event,
		Ack:     func() error { return message.Ack(false) },
		Reject:  func(requeue bool) error { return message.Nack(false, requeue) },
	}, nil
}

func (b *Broker) QueueDepth(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	channel, err := b.connection.Channel()
	if err != nil {
		return 0, fmt.Errorf("open queue inspection channel: %w", err)
	}
	defer func() { _ = channel.Close() }()
	queue, err := channel.QueueDeclarePassive(queueName, true, false, false, false, nil)
	if err != nil {
		return 0, fmt.Errorf("inspect queue: %w", err)
	}
	return queue.Messages, nil
}

func (b *Broker) Ping(ctx context.Context) error {
	if b.connection.IsClosed() {
		return fmt.Errorf("connection is closed")
	}
	return b.channel.ExchangeDeclarePassive(exchangeName, "direct", true, false, false, false, nil)
}

func (b *Broker) Close() error {
	if b.channel != nil {
		_ = b.channel.Close()
	}
	if b.connection != nil {
		return b.connection.Close()
	}
	return nil
}

var _ broker.Publisher = (*Broker)(nil)
var _ broker.Consumer = (*Broker)(nil)
