package domain

import (
	"errors"
	"testing"
	"time"
)

func TestJobStatusConstants(t *testing.T) {
	if JobStatusProcessing != "PROCESSING" {
		t.Errorf("expected PROCESSING, got %s", JobStatusProcessing)
	}
	if JobStatusCompleted != "COMPLETED" {
		t.Errorf("expected COMPLETED, got %s", JobStatusCompleted)
	}
	if JobStatusError != "ERROR" {
		t.Errorf("expected ERROR, got %s", JobStatusError)
	}
}

func TestDomainErrors_AreDistinct(t *testing.T) {
	errs := []error{ErrJobNotFound, ErrInvalidInput, ErrLLMGuardrail}

	for i, a := range errs {
		for j, b := range errs {
			if i != j && errors.Is(a, b) {
				t.Errorf("errors[%d] and errors[%d] should be distinct", i, j)
			}
		}
	}
}

func TestDomainErrors_AreNotNil(t *testing.T) {
	if ErrJobNotFound == nil {
		t.Error("ErrJobNotFound should not be nil")
	}
	if ErrInvalidInput == nil {
		t.Error("ErrInvalidInput should not be nil")
	}
	if ErrLLMGuardrail == nil {
		t.Error("ErrLLMGuardrail should not be nil")
	}
}

func TestProcessingJob_ZeroValue(t *testing.T) {
	var job ProcessingJob

	if job.ProcessID != "" {
		t.Error("zero-value ProcessID should be empty")
	}
	if job.Status != "" {
		t.Error("zero-value Status should be empty")
	}
	if job.Analysis != nil {
		t.Error("zero-value Analysis should be nil")
	}
	if !job.StartedAt.IsZero() {
		t.Error("zero-value StartedAt should be zero time")
	}
}

func TestProcessingJob_FieldAssignment(t *testing.T) {
	now := time.Now().UTC()
	analysis := &Analysis{
		Components:      []string{"svc-a", "db"},
		Risks:           []string{"no auth"},
		Recommendations: []string{"add mTLS"},
	}

	job := &ProcessingJob{
		ProcessID:   "proc-123",
		Status:      JobStatusProcessing,
		LLMResponse: "raw response",
		Analysis:    analysis,
		ErrorMsg:    "",
		StartedAt:   now,
	}

	if job.ProcessID != "proc-123" {
		t.Errorf("expected proc-123, got %s", job.ProcessID)
	}
	if job.Status != JobStatusProcessing {
		t.Errorf("expected PROCESSING, got %s", job.Status)
	}
	if job.Analysis != analysis {
		t.Error("analysis pointer mismatch")
	}
	if job.StartedAt != now {
		t.Error("StartedAt mismatch")
	}
}

func TestAnalysis_Fields(t *testing.T) {
	a := &Analysis{
		Components:      []string{"svc-a", "svc-b", "db"},
		Risks:           []string{"no auth", "open port"},
		Recommendations: []string{"add mTLS", "close port 22"},
	}

	if len(a.Components) != 3 {
		t.Errorf("expected 3 components, got %d", len(a.Components))
	}
	if len(a.Risks) != 2 {
		t.Errorf("expected 2 risks, got %d", len(a.Risks))
	}
	if len(a.Recommendations) != 2 {
		t.Errorf("expected 2 recommendations, got %d", len(a.Recommendations))
	}
}
