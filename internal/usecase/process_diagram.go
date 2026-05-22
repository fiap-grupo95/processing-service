package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fiap/secure-systems/processing-service/internal/domain"
	"github.com/fiap/secure-systems/processing-service/internal/logging"
)

type ProcessDiagramInput struct {
	ProcessID   string
	S3Key       string
	ContentType string
}

type processingEvent struct {
	ProcessID string `json:"process_id"`
	Event     string `json:"event"`
	ErrorMsg  string `json:"error,omitempty"`
}

type reportPayload struct {
	ProcessID   string           `json:"process_id"`
	Analysis    *domain.Analysis `json:"analysis"`
	RawResponse string           `json:"raw_response"`
}

type ProcessDiagramUseCase struct {
	repo            JobRepository
	downloader      DiagramDownloader
	llm             LLMClient
	publisher       EventPublisher
	processingTopic string
	reportQueue     string
}

func NewProcessDiagramUseCase(
	repo JobRepository,
	downloader DiagramDownloader,
	llm LLMClient,
	publisher EventPublisher,
	processingTopic, reportQueue string,
) *ProcessDiagramUseCase {
	return &ProcessDiagramUseCase{
		repo:            repo,
		downloader:      downloader,
		llm:             llm,
		publisher:       publisher,
		processingTopic: processingTopic,
		reportQueue:     reportQueue,
	}
}

func (uc *ProcessDiagramUseCase) Execute(ctx context.Context, in ProcessDiagramInput) error {
	log := logging.LoggerWithContext(ctx).With().Str("process_id", in.ProcessID).Logger()

	// 1. Persiste o job com status PROCESSING
	job := &domain.ProcessingJob{
		ProcessID: in.ProcessID,
		Status:    domain.JobStatusProcessing,
		StartedAt: time.Now().UTC(),
	}
	if err := uc.repo.Save(ctx, job); err != nil {
		return fmt.Errorf("save job: %w", err)
	}

	// 2. Notifica orquestrador: processamento iniciado
	if err := uc.publishProcessingEvent(ctx, in.ProcessID, "processing_started", ""); err != nil {
		log.Warn().Err(err).Msg("failed to publish processing_started")
	}

	// 3. Download do diagrama do MinIO
	defer logging.StartSegment(ctx, "ProcessDiagram.StorageDownload")()
	imageData, contentType, err := uc.downloader.Download(ctx, in.S3Key)
	if err != nil {
		return uc.failJob(ctx, in.ProcessID, fmt.Sprintf("download diagram: %v", err))
	}
	if contentType == "" {
		contentType = in.ContentType
	}
	log.Info().Int("bytes", len(imageData)).Msg("diagram downloaded")

	// 4. Análise pela IA
	defer logging.StartSegment(ctx, "ProcessDiagram.LLMAnalysis")()
	analysis, rawResponse, err := uc.llm.Analyze(ctx, imageData, contentType)
	if err != nil {
		return uc.failJob(ctx, in.ProcessID, fmt.Sprintf("llm analysis: %v", err))
	}

	// 5. Persiste resultado completo no DynamoDB
	defer logging.StartSegment(ctx, "ProcessDiagram.DynamoSave")()
	if err := uc.repo.UpdateCompleted(ctx, in.ProcessID, rawResponse, analysis); err != nil {
		return uc.failJob(ctx, in.ProcessID, fmt.Sprintf("update completed: %v", err))
	}

	// 6. Publica análise na report.queue para o report-service
	payload, _ := json.Marshal(reportPayload{
		ProcessID:   in.ProcessID,
		Analysis:    analysis,
		RawResponse: rawResponse,
	})
	if err := uc.publisher.PublishToQueue(ctx, uc.reportQueue, payload); err != nil {
		return uc.failJob(ctx, in.ProcessID, fmt.Sprintf("publish to report queue: %v", err))
	}

	log.Info().Msg("diagram processed successfully")
	return nil
}

func (uc *ProcessDiagramUseCase) failJob(ctx context.Context, processID, errMsg string) error {
	logging.LoggerWithContext(ctx).Error().
		Str("process_id", processID).Str("reason", errMsg).Msg("processing failed")

	if err := uc.repo.UpdateError(ctx, processID, errMsg); err != nil {
		logging.LoggerWithContext(ctx).Error().Err(err).Msg("failed to persist error state")
	}
	if err := uc.publishProcessingEvent(ctx, processID, "processing_error", errMsg); err != nil {
		logging.LoggerWithContext(ctx).Error().Err(err).Msg("failed to publish processing_error")
	}

	return fmt.Errorf("%s", errMsg)
}

func (uc *ProcessDiagramUseCase) publishProcessingEvent(ctx context.Context, processID, event, errMsg string) error {
	payload, _ := json.Marshal(processingEvent{
		ProcessID: processID,
		Event:     event,
		ErrorMsg:  errMsg,
	})
	return uc.publisher.PublishToExchange(ctx, uc.processingTopic, payload)
}
