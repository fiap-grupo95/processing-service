package usecase

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/fiap/secure-systems/processing-service/internal/domain"
)

// ──────────────────────────────────────────────
// Mocks
// ──────────────────────────────────────────────

type mockJobRepository struct {
	saveFunc            func(ctx context.Context, job *domain.ProcessingJob) error
	updateCompletedFunc func(ctx context.Context, processID, llmResponse string, analysis *domain.Analysis) error
	updateErrorFunc     func(ctx context.Context, processID, errMsg string) error
}

func (m *mockJobRepository) Save(ctx context.Context, job *domain.ProcessingJob) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, job)
	}
	return nil
}

func (m *mockJobRepository) UpdateCompleted(ctx context.Context, processID, llmResponse string, analysis *domain.Analysis) error {
	if m.updateCompletedFunc != nil {
		return m.updateCompletedFunc(ctx, processID, llmResponse, analysis)
	}
	return nil
}

func (m *mockJobRepository) UpdateError(ctx context.Context, processID, errMsg string) error {
	if m.updateErrorFunc != nil {
		return m.updateErrorFunc(ctx, processID, errMsg)
	}
	return nil
}

type mockDiagramDownloader struct {
	downloadFunc func(ctx context.Context, key string) ([]byte, string, error)
}

func (m *mockDiagramDownloader) Download(ctx context.Context, key string) ([]byte, string, error) {
	if m.downloadFunc != nil {
		return m.downloadFunc(ctx, key)
	}
	return []byte("fake-image-data"), "image/png", nil
}

type mockLLMClient struct {
	analyzeFunc func(ctx context.Context, imageData []byte, contentType string) (*domain.Analysis, string, error)
}

func (m *mockLLMClient) Analyze(ctx context.Context, imageData []byte, contentType string) (*domain.Analysis, string, error) {
	if m.analyzeFunc != nil {
		return m.analyzeFunc(ctx, imageData, contentType)
	}
	return &domain.Analysis{
		Components:      []string{"svc-a"},
		Risks:           []string{"risk-1"},
		Recommendations: []string{"rec-1"},
	}, "raw-llm-response", nil
}

type mockEventPublisher struct {
	publishToQueueFunc    func(ctx context.Context, queue string, payload []byte) error
	publishToExchangeFunc func(ctx context.Context, exchange string, payload []byte) error
}

func (m *mockEventPublisher) PublishToQueue(ctx context.Context, queue string, payload []byte) error {
	if m.publishToQueueFunc != nil {
		return m.publishToQueueFunc(ctx, queue, payload)
	}
	return nil
}

func (m *mockEventPublisher) PublishToExchange(ctx context.Context, exchange string, payload []byte) error {
	if m.publishToExchangeFunc != nil {
		return m.publishToExchangeFunc(ctx, exchange, payload)
	}
	return nil
}

// ──────────────────────────────────────────────
// Helper
// ──────────────────────────────────────────────

func newUseCase(repo JobRepository, dl DiagramDownloader, llm LLMClient, pub EventPublisher) *ProcessDiagramUseCase {
	return NewProcessDiagramUseCase(repo, dl, llm, pub, "processing.topic", "report.queue")
}

func defaultInput() ProcessDiagramInput {
	return ProcessDiagramInput{
		ProcessID:   "proc-123",
		S3Key:       "diagrams/proc-123.png",
		ContentType: "image/png",
	}
}

// ──────────────────────────────────────────────
// Testes de Execute
// ──────────────────────────────────────────────

func TestExecute_HappyPath(t *testing.T) {
	uc := newUseCase(&mockJobRepository{}, &mockDiagramDownloader{}, &mockLLMClient{}, &mockEventPublisher{})

	if err := uc.Execute(context.Background(), defaultInput()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecute_PersistsJobWithProcessingStatus(t *testing.T) {
	var savedJob *domain.ProcessingJob
	repo := &mockJobRepository{
		saveFunc: func(_ context.Context, job *domain.ProcessingJob) error {
			savedJob = job
			return nil
		},
	}

	uc := newUseCase(repo, &mockDiagramDownloader{}, &mockLLMClient{}, &mockEventPublisher{})
	_ = uc.Execute(context.Background(), defaultInput())

	if savedJob == nil {
		t.Fatal("expected job to be saved")
	}
	if savedJob.Status != domain.JobStatusProcessing {
		t.Errorf("expected PROCESSING status, got %s", savedJob.Status)
	}
	if savedJob.ProcessID != "proc-123" {
		t.Errorf("expected proc-123, got %s", savedJob.ProcessID)
	}
	if savedJob.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}

func TestExecute_RepoSaveError_ReturnsError(t *testing.T) {
	repo := &mockJobRepository{
		saveFunc: func(_ context.Context, _ *domain.ProcessingJob) error {
			return errors.New("db unavailable")
		},
	}

	uc := newUseCase(repo, &mockDiagramDownloader{}, &mockLLMClient{}, &mockEventPublisher{})

	err := uc.Execute(context.Background(), defaultInput())
	if err == nil {
		t.Error("expected error when repo.Save fails")
	}
}

func TestExecute_DownloadError_CallsFailJob(t *testing.T) {
	dl := &mockDiagramDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, string, error) {
			return nil, "", errors.New("minio unavailable")
		},
	}

	var updateErrorCalled bool
	repo := &mockJobRepository{
		updateErrorFunc: func(_ context.Context, _ string, _ string) error {
			updateErrorCalled = true
			return nil
		},
	}

	uc := newUseCase(repo, dl, &mockLLMClient{}, &mockEventPublisher{})

	err := uc.Execute(context.Background(), defaultInput())
	if err == nil {
		t.Error("expected error when download fails")
	}
	if !updateErrorCalled {
		t.Error("expected UpdateError to be called after download failure")
	}
}

func TestExecute_LLMError_CallsFailJob(t *testing.T) {
	llm := &mockLLMClient{
		analyzeFunc: func(_ context.Context, _ []byte, _ string) (*domain.Analysis, string, error) {
			return nil, "", errors.New("llm timeout")
		},
	}

	var updateErrorCalled bool
	repo := &mockJobRepository{
		updateErrorFunc: func(_ context.Context, _ string, _ string) error {
			updateErrorCalled = true
			return nil
		},
	}

	uc := newUseCase(repo, &mockDiagramDownloader{}, llm, &mockEventPublisher{})

	err := uc.Execute(context.Background(), defaultInput())
	if err == nil {
		t.Error("expected error when LLM fails")
	}
	if !updateErrorCalled {
		t.Error("expected UpdateError to be called after LLM failure")
	}
}

func TestExecute_UpdateCompletedError_CallsFailJob(t *testing.T) {
	var updateErrorCalled bool
	repo := &mockJobRepository{
		updateCompletedFunc: func(_ context.Context, _ string, _ string, _ *domain.Analysis) error {
			return errors.New("dynamo write failed")
		},
		updateErrorFunc: func(_ context.Context, _ string, _ string) error {
			updateErrorCalled = true
			return nil
		},
	}

	uc := newUseCase(repo, &mockDiagramDownloader{}, &mockLLMClient{}, &mockEventPublisher{})

	err := uc.Execute(context.Background(), defaultInput())
	if err == nil {
		t.Error("expected error when UpdateCompleted fails")
	}
	if !updateErrorCalled {
		t.Error("expected UpdateError to be called after UpdateCompleted failure")
	}
}

func TestExecute_PublishToQueueError_CallsFailJob(t *testing.T) {
	pub := &mockEventPublisher{
		publishToQueueFunc: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("rabbit queue down")
		},
	}

	var updateErrorCalled bool
	repo := &mockJobRepository{
		updateErrorFunc: func(_ context.Context, _ string, _ string) error {
			updateErrorCalled = true
			return nil
		},
	}

	uc := newUseCase(repo, &mockDiagramDownloader{}, &mockLLMClient{}, pub)

	err := uc.Execute(context.Background(), defaultInput())
	if err == nil {
		t.Error("expected error when PublishToQueue fails")
	}
	if !updateErrorCalled {
		t.Error("expected UpdateError to be called after PublishToQueue failure")
	}
}

func TestExecute_ProcessingStartedExchangeFailure_OnlyWarns(t *testing.T) {
	// Falha no exchange de processing_started só deve gerar warning, não falhar o fluxo.
	pub := &mockEventPublisher{
		publishToExchangeFunc: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("exchange unavailable")
		},
	}

	uc := newUseCase(&mockJobRepository{}, &mockDiagramDownloader{}, &mockLLMClient{}, pub)

	// PublishToQueue (report queue) não está mockada para erro, então o fluxo deve terminar ok.
	// PublishToExchange falha, mas isso apenas gera warning no passo 2.
	// No passo 6, o PublishToQueue usa publishToQueueFunc (nil = sem erro).
	err := uc.Execute(context.Background(), defaultInput())
	if err != nil {
		t.Errorf("expected no error when only exchange publish fails: %v", err)
	}
}

func TestExecute_FallbackContentType(t *testing.T) {
	// Quando o downloader retorna contentType vazio, deve usar o da entrada.
	dl := &mockDiagramDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, string, error) {
			return []byte("img"), "", nil // contentType vazio
		},
	}

	var capturedContentType string
	llm := &mockLLMClient{
		analyzeFunc: func(_ context.Context, _ []byte, ct string) (*domain.Analysis, string, error) {
			capturedContentType = ct
			return &domain.Analysis{
				Components:      []string{"c1"},
				Risks:           []string{"r1"},
				Recommendations: []string{"rec1"},
			}, "raw", nil
		},
	}

	uc := newUseCase(&mockJobRepository{}, dl, llm, &mockEventPublisher{})

	err := uc.Execute(context.Background(), ProcessDiagramInput{
		ProcessID:   "proc-123",
		S3Key:       "diagrams/proc-123.png",
		ContentType: "image/jpeg", // deve ser usado como fallback
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if capturedContentType != "image/jpeg" {
		t.Errorf("expected fallback contentType 'image/jpeg', got %q", capturedContentType)
	}
}

func TestExecute_UsesDownloadedContentType(t *testing.T) {
	// Quando o downloader retorna contentType preenchido, deve usá-lo (não o da entrada).
	dl := &mockDiagramDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, string, error) {
			return []byte("img"), "image/webp", nil
		},
	}

	var capturedContentType string
	llm := &mockLLMClient{
		analyzeFunc: func(_ context.Context, _ []byte, ct string) (*domain.Analysis, string, error) {
			capturedContentType = ct
			return &domain.Analysis{
				Components:      []string{"c1"},
				Risks:           []string{"r1"},
				Recommendations: []string{"rec1"},
			}, "raw", nil
		},
	}

	uc := newUseCase(&mockJobRepository{}, dl, llm, &mockEventPublisher{})

	err := uc.Execute(context.Background(), ProcessDiagramInput{
		ProcessID:   "proc-123",
		S3Key:       "diagrams/proc-123.png",
		ContentType: "image/png", // deve ser ignorado pois o downloader retornou webp
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if capturedContentType != "image/webp" {
		t.Errorf("expected 'image/webp' from downloader, got %q", capturedContentType)
	}
}

func TestExecute_ReportQueuePayloadContainsProcessID(t *testing.T) {
	var publishedPayload []byte
	pub := &mockEventPublisher{
		publishToQueueFunc: func(_ context.Context, _ string, payload []byte) error {
			publishedPayload = payload
			return nil
		},
	}

	uc := newUseCase(&mockJobRepository{}, &mockDiagramDownloader{}, &mockLLMClient{}, pub)

	_ = uc.Execute(context.Background(), defaultInput())

	if publishedPayload == nil {
		t.Fatal("expected payload to be published to report queue")
	}
	if !bytes.Contains(publishedPayload, []byte("proc-123")) {
		t.Errorf("expected published payload to contain process ID, got: %s", publishedPayload)
	}
}

func TestNewProcessDiagramUseCase_SetsFields(t *testing.T) {
	repo := &mockJobRepository{}
	dl := &mockDiagramDownloader{}
	llm := &mockLLMClient{}
	pub := &mockEventPublisher{}

	uc := NewProcessDiagramUseCase(repo, dl, llm, pub, "my.topic", "my.queue")

	if uc.processingTopic != "my.topic" {
		t.Errorf("expected my.topic, got %s", uc.processingTopic)
	}
	if uc.reportQueue != "my.queue" {
		t.Errorf("expected my.queue, got %s", uc.reportQueue)
	}
}

func TestExecute_FailJob_UpdateErrorAlsoFails_StillReturnsOriginalError(t *testing.T) {
	// Simula o cenário mais adverso: download falha E updateError também falha.
	// Deve retornar o erro original de download, sem panic.
	dl := &mockDiagramDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, string, error) {
			return nil, "", errors.New("download failed")
		},
	}
	repo := &mockJobRepository{
		updateErrorFunc: func(_ context.Context, _ string, _ string) error {
			return errors.New("dynamo also failed")
		},
	}

	uc := newUseCase(repo, dl, &mockLLMClient{}, &mockEventPublisher{})

	err := uc.Execute(context.Background(), defaultInput())
	if err == nil {
		t.Error("expected error when both download and UpdateError fail")
	}
}

func TestExecute_FailJob_PublishErrorEventAlsoFails(t *testing.T) {
	// Cobre o branch onde publishProcessingEvent dentro de failJob também falha.
	dl := &mockDiagramDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, string, error) {
			return nil, "", errors.New("download failed")
		},
	}
	pub := &mockEventPublisher{
		publishToExchangeFunc: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("exchange unavailable")
		},
	}

	uc := newUseCase(&mockJobRepository{}, dl, &mockLLMClient{}, pub)

	err := uc.Execute(context.Background(), defaultInput())
	if err == nil {
		t.Error("expected error when download fails, even with exchange errors")
	}
}

