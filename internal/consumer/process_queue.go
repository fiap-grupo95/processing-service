package consumer

import (
	"context"
	"encoding/json"

	"github.com/fiap/secure-systems/processing-service/internal/logging"
	"github.com/fiap/secure-systems/processing-service/internal/usecase"
	"github.com/newrelic/go-agent/v3/newrelic"
	amqp "github.com/rabbitmq/amqp091-go"
)

type processMessage struct {
	ProcessID   string `json:"process_id"`
	S3Key       string `json:"s3_key"`
	ContentType string `json:"content_type"`
}

// diagramProcessor abstrai o caso de uso para permitir testes com mocks.
type diagramProcessor interface {
	Execute(ctx context.Context, in usecase.ProcessDiagramInput) error
}

type ProcessQueueConsumer struct {
	uc    diagramProcessor
	nrApp *newrelic.Application
}

func NewProcessQueueConsumer(
	uc diagramProcessor,
	nrApp *newrelic.Application,
) *ProcessQueueConsumer {
	return &ProcessQueueConsumer{uc: uc, nrApp: nrApp}
}

func (c *ProcessQueueConsumer) Run(ctx context.Context, deliveries <-chan amqp.Delivery) {
	logging.Logger().Info().Msg("process queue consumer started")
	for {
		select {
		case <-ctx.Done():
			logging.Logger().Info().Msg("process queue consumer stopped")
			return
		case d, ok := <-deliveries:
			if !ok {
				logging.Logger().Warn().Msg("process queue channel closed")
				return
			}
			c.handle(d)
		}
	}
}

func (c *ProcessQueueConsumer) handle(d amqp.Delivery) {
	txn := c.nrApp.StartTransaction("consumer/process-queue")
	defer txn.End()

	var msg processMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logging.Logger().Error().Err(err).Msg("invalid process queue message")
		txn.NoticeError(err)
		d.Nack(false, false)
		return
	}

	if msg.ProcessID == "" || msg.S3Key == "" {
		logging.Logger().Error().
			Str("process_id", msg.ProcessID).
			Str("s3_key", msg.S3Key).
			Msg("process message missing required fields")
		d.Nack(false, false)
		return
	}

	ctx := newrelic.NewContext(context.Background(), txn)
	txn.AddAttribute("process_id", msg.ProcessID)

	if err := c.uc.Execute(ctx, usecase.ProcessDiagramInput{
		ProcessID:   msg.ProcessID,
		S3Key:       msg.S3Key,
		ContentType: msg.ContentType,
	}); err != nil {
		logging.LoggerWithContext(ctx).Error().
			Str("process_id", msg.ProcessID).Err(err).Msg("process diagram failed")
		txn.NoticeError(err)
		// Nack sem requeue: o use case já atualizou o status para ERRO e notificou o orquestrador
		d.Nack(false, false)
		return
	}

	d.Ack(false)
}
