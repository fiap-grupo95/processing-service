package consumer

import (
	"context"
	"encoding/json"

	"github.com/fiap/secure-systems/processing-service/internal/usecase"
	"github.com/newrelic/go-agent/v3/newrelic"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type processMessage struct {
	ProcessID   string `json:"process_id"`
	S3Key       string `json:"s3_key"`
	ContentType string `json:"content_type"`
}

type ProcessQueueConsumer struct {
	uc    *usecase.ProcessDiagramUseCase
	nrApp *newrelic.Application
	log   *zap.Logger
}

func NewProcessQueueConsumer(
	uc *usecase.ProcessDiagramUseCase,
	nrApp *newrelic.Application,
	log *zap.Logger,
) *ProcessQueueConsumer {
	return &ProcessQueueConsumer{uc: uc, nrApp: nrApp, log: log}
}

func (c *ProcessQueueConsumer) Run(ctx context.Context, deliveries <-chan amqp.Delivery) {
	c.log.Info("process queue consumer started")
	for {
		select {
		case <-ctx.Done():
			c.log.Info("process queue consumer stopped")
			return
		case d, ok := <-deliveries:
			if !ok {
				c.log.Warn("process queue channel closed")
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
		c.log.Error("invalid process queue message", zap.Error(err))
		txn.NoticeError(err)
		d.Nack(false, false) // descarta — payload inválido não vai melhorar com requeue
		return
	}

	if msg.ProcessID == "" || msg.S3Key == "" {
		c.log.Error("process message missing required fields",
			zap.String("processId", msg.ProcessID),
			zap.String("s3Key", msg.S3Key),
		)
		d.Nack(false, false)
		return
	}

	ctx := newrelic.NewContext(context.Background(), txn)
	txn.AddAttribute("processId", msg.ProcessID)

	if err := c.uc.Execute(ctx, usecase.ProcessDiagramInput{
		ProcessID:   msg.ProcessID,
		S3Key:       msg.S3Key,
		ContentType: msg.ContentType,
	}); err != nil {
		c.log.Error("process diagram failed",
			zap.String("processId", msg.ProcessID),
			zap.Error(err),
		)
		txn.NoticeError(err)
		// Nack sem requeue: o use case já atualizou o status para ERRO e notificou o orquestrador
		d.Nack(false, false)
		return
	}

	d.Ack(false)
}
