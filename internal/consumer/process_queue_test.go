package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fiap/secure-systems/processing-service/internal/usecase"
	"github.com/newrelic/go-agent/v3/newrelic"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// ──────────────────────────────────────────────
// Mocks
// ──────────────────────────────────────────────

type mockProcessor struct {
	executeFunc func(ctx context.Context, in usecase.ProcessDiagramInput) error
}

func (m *mockProcessor) Execute(ctx context.Context, in usecase.ProcessDiagramInput) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, in)
	}
	return nil
}

// mockAcknowledger implementa amqp.Acknowledger para testes.
type mockAcknowledger struct {
	ackCalled  bool
	nackCalled bool
}

func (m *mockAcknowledger) Ack(_ uint64, _ bool) error {
	m.ackCalled = true
	return nil
}

func (m *mockAcknowledger) Nack(_ uint64, _ bool, _ bool) error {
	m.nackCalled = true
	return nil
}

func (m *mockAcknowledger) Reject(_ uint64, _ bool) error {
	return nil
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func newTestConsumer(proc diagramProcessor) *ProcessQueueConsumer {
	nrApp, _ := newrelic.NewApplication(newrelic.ConfigEnabled(false))
	return NewProcessQueueConsumer(proc, nrApp, zap.NewNop())
}

func newDelivery(ack *mockAcknowledger, body []byte) amqp.Delivery {
	return amqp.Delivery{
		Acknowledger: ack,
		Body:         body,
	}
}

// ──────────────────────────────────────────────
// Testes de handle
// ──────────────────────────────────────────────

func TestHandle_ValidMessage_CallsAck(t *testing.T) {
	ack := &mockAcknowledger{}
	proc := &mockProcessor{}
	consumer := newTestConsumer(proc)

	body := []byte(`{"process_id":"proc-123","s3_key":"diagrams/proc-123.png","content_type":"image/png"}`)
	consumer.handle(newDelivery(ack, body))

	if !ack.ackCalled {
		t.Error("expected Ack to be called for valid message")
	}
	if ack.nackCalled {
		t.Error("expected Nack NOT to be called for valid message")
	}
}

func TestHandle_InvalidJSON_CallsNack(t *testing.T) {
	ack := &mockAcknowledger{}
	proc := &mockProcessor{}
	consumer := newTestConsumer(proc)

	consumer.handle(newDelivery(ack, []byte("not-json")))

	if !ack.nackCalled {
		t.Error("expected Nack to be called for invalid JSON")
	}
	if ack.ackCalled {
		t.Error("expected Ack NOT to be called for invalid JSON")
	}
}

func TestHandle_MissingProcessID_CallsNack(t *testing.T) {
	ack := &mockAcknowledger{}
	proc := &mockProcessor{}
	consumer := newTestConsumer(proc)

	body := []byte(`{"process_id":"","s3_key":"diagrams/proc-123.png"}`)
	consumer.handle(newDelivery(ack, body))

	if !ack.nackCalled {
		t.Error("expected Nack for message with empty process_id")
	}
}

func TestHandle_MissingS3Key_CallsNack(t *testing.T) {
	ack := &mockAcknowledger{}
	proc := &mockProcessor{}
	consumer := newTestConsumer(proc)

	body := []byte(`{"process_id":"proc-123","s3_key":""}`)
	consumer.handle(newDelivery(ack, body))

	if !ack.nackCalled {
		t.Error("expected Nack for message with empty s3_key")
	}
}

func TestHandle_UseCaseError_CallsNack(t *testing.T) {
	ack := &mockAcknowledger{}
	proc := &mockProcessor{
		executeFunc: func(_ context.Context, _ usecase.ProcessDiagramInput) error {
			return errors.New("use case failed")
		},
	}
	consumer := newTestConsumer(proc)

	body := []byte(`{"process_id":"proc-123","s3_key":"diagrams/proc-123.png"}`)
	consumer.handle(newDelivery(ack, body))

	if !ack.nackCalled {
		t.Error("expected Nack when use case returns error")
	}
	if ack.ackCalled {
		t.Error("expected Ack NOT to be called when use case fails")
	}
}

func TestHandle_PassesCorrectInputToUseCase(t *testing.T) {
	ack := &mockAcknowledger{}

	var capturedInput usecase.ProcessDiagramInput
	proc := &mockProcessor{
		executeFunc: func(_ context.Context, in usecase.ProcessDiagramInput) error {
			capturedInput = in
			return nil
		},
	}
	consumer := newTestConsumer(proc)

	body := []byte(`{"process_id":"proc-456","s3_key":"diagrams/arch.png","content_type":"image/png"}`)
	consumer.handle(newDelivery(ack, body))

	if capturedInput.ProcessID != "proc-456" {
		t.Errorf("expected ProcessID=proc-456, got %s", capturedInput.ProcessID)
	}
	if capturedInput.S3Key != "diagrams/arch.png" {
		t.Errorf("expected S3Key=diagrams/arch.png, got %s", capturedInput.S3Key)
	}
	if capturedInput.ContentType != "image/png" {
		t.Errorf("expected ContentType=image/png, got %s", capturedInput.ContentType)
	}
}

func TestHandle_EmptyBody_CallsNack(t *testing.T) {
	ack := &mockAcknowledger{}
	proc := &mockProcessor{}
	consumer := newTestConsumer(proc)

	consumer.handle(newDelivery(ack, []byte("")))

	if !ack.nackCalled {
		t.Error("expected Nack for empty body")
	}
}

// ──────────────────────────────────────────────
// Testes de Run
// ──────────────────────────────────────────────

func TestRun_StopsOnContextCancel(t *testing.T) {
	proc := &mockProcessor{}
	consumer := newTestConsumer(proc)

	ctx, cancel := context.WithCancel(context.Background())
	deliveries := make(chan amqp.Delivery)

	done := make(chan struct{})
	go func() {
		consumer.Run(ctx, deliveries)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// ok: consumer parou após cancelamento do contexto
	case <-time.After(2 * time.Second):
		t.Error("consumer did not stop after context cancellation")
	}
}

func TestRun_StopsWhenChannelClosed(t *testing.T) {
	proc := &mockProcessor{}
	consumer := newTestConsumer(proc)

	ctx := context.Background()
	deliveries := make(chan amqp.Delivery)

	done := make(chan struct{})
	go func() {
		consumer.Run(ctx, deliveries)
		close(done)
	}()

	close(deliveries)

	select {
	case <-done:
		// ok: consumer parou após fechamento do canal
	case <-time.After(2 * time.Second):
		t.Error("consumer did not stop after channel was closed")
	}
}

func TestRun_ProcessesMessage(t *testing.T) {
	executed := make(chan struct{}, 1)
	proc := &mockProcessor{
		executeFunc: func(_ context.Context, _ usecase.ProcessDiagramInput) error {
			executed <- struct{}{}
			return nil
		},
	}
	consumer := newTestConsumer(proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deliveries := make(chan amqp.Delivery, 1)

	ack := &mockAcknowledger{}
	body := []byte(`{"process_id":"proc-123","s3_key":"diagrams/proc-123.png"}`)
	deliveries <- newDelivery(ack, body)

	go consumer.Run(ctx, deliveries)

	select {
	case <-executed:
		// ok: Execute foi chamado
	case <-time.After(2 * time.Second):
		t.Error("expected use case Execute to be called within 2 seconds")
	}
}
