package app

import (
	"context"
	"log/slog"
	"time"
)

type OutboxWorker struct {
	repository CabinetRepository
	publisher  EventPublisher
	logger     *slog.Logger
}

func NewOutboxWorker(repository CabinetRepository, publisher EventPublisher, logger *slog.Logger) *OutboxWorker {
	return &OutboxWorker{repository: repository, publisher: publisher, logger: logger}
}

func (w *OutboxWorker) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.proccessBatch(ctx)
		}
	}

}

func (w *OutboxWorker) proccessBatch(ctx context.Context) {
	pendentEvents, err := w.repository.ListUnpublishedOutboxEvents(ctx, 25)
	if err != nil {
		w.logger.Error("failed to list unpublished outbox events", "error", err)
		return
	}

	for _, event := range pendentEvents {
		err := w.publisher.Publish(ctx, event.EventType, event.Payload)
		if err != nil {
			w.logger.Error("failed to publish event", "error", err)
			continue
		} else {
			err := w.repository.MarkOutboxEventPublished(ctx, event.ID)
			if err != nil {
				w.logger.Error("failed to mark event as published", "error", err)
			}
		}

	}
}
