package publisher

import (
	"context"
	"log/slog"
)

// LogPublisher it's a simple implementation of EventPublisher to dev/logs
type LogPublisher struct {
	logger *slog.Logger
}

func NewLogPublisher(logger *slog.Logger) *LogPublisher {
	return &LogPublisher{logger: logger}
}

func (p *LogPublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	p.logger.InfoContext(ctx, "outbox event published", "eventType", eventType, "payload", string(payload))
	return nil
}
