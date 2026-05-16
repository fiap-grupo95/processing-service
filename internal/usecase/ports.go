package usecase

import (
	"context"

	"github.com/fiap/secure-systems/processing-service/internal/domain"
)

type JobRepository interface {
	Save(ctx context.Context, job *domain.ProcessingJob) error
	UpdateCompleted(ctx context.Context, processID, llmResponse string, analysis *domain.Analysis) error
	UpdateError(ctx context.Context, processID, errMsg string) error
}

type DiagramDownloader interface {
	Download(ctx context.Context, key string) (content []byte, contentType string, err error)
}

type LLMClient interface {
	Analyze(ctx context.Context, imageData []byte, contentType string) (analysis *domain.Analysis, rawResponse string, err error)
}

type EventPublisher interface {
	PublishToQueue(ctx context.Context, queue string, payload []byte) error
	PublishToExchange(ctx context.Context, exchange string, payload []byte) error
}
