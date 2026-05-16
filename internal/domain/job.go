package domain

import (
	"errors"
	"time"
)

type JobStatus string

const (
	JobStatusProcessing JobStatus = "PROCESSING"
	JobStatusCompleted  JobStatus = "COMPLETED"
	JobStatusError      JobStatus = "ERROR"
)

var (
	ErrJobNotFound  = errors.New("processing job not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrLLMGuardrail = errors.New("llm response failed guardrail validation")
)

// Analysis é o resultado estruturado extraído do diagrama pela IA.
type Analysis struct {
	Components      []string `json:"components"`
	Risks           []string `json:"risks"`
	Recommendations []string `json:"recommendations"`
}

// ProcessingJob representa uma execução de análise de diagrama.
type ProcessingJob struct {
	ProcessID   string
	Status      JobStatus
	PromptUsed  string
	LLMResponse string
	Analysis    *Analysis
	ErrorMsg    string
	StartedAt   time.Time
	CompletedAt time.Time
}
