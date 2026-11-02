package broker

import (
	"context"

	"github.com/example/gophprofile/internal/domain"
)

type EventType string

const (
	EventUpload EventType = "avatar.uploaded"
	EventDelete EventType = "avatar.deleted"
)

type Event struct {
	Type   EventType
	Upload domain.AvatarUploadEvent
	Delete domain.AvatarDeleteEvent
}

type Delivery struct {
	Event  Event
	Ack    func() error
	Reject func(requeue bool) error
}

type Publisher interface {
	PublishUpload(ctx context.Context, event domain.AvatarUploadEvent) error
	PublishDelete(ctx context.Context, event domain.AvatarDeleteEvent) error
	Ping(ctx context.Context) error
	Close() error
}

type Consumer interface {
	Consume(ctx context.Context) (<-chan Delivery, error)
	Ping(ctx context.Context) error
	Close() error
}
