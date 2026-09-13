package app

import "context"

// EventPublisher defines the output port for dispatching outbox events to a broker or message queue
type EventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}
