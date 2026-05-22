package ai

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/fiap/secure-systems/processing-service/internal/domain"
)

// ──────────────────────────────────────────────
// toMediaType
// ──────────────────────────────────────────────

func TestToMediaType_Supported(t *testing.T) {
	cases := []struct {
		contentType string
		expected    anthropic.Base64ImageSourceMediaType
	}{
		{"image/jpeg", anthropic.Base64ImageSourceMediaTypeImageJPEG},
		{"image/jpeg; charset=utf-8", anthropic.Base64ImageSourceMediaTypeImageJPEG},
		{"image/png", anthropic.Base64ImageSourceMediaTypeImagePNG},
		{"image/gif", anthropic.Base64ImageSourceMediaTypeImageGIF},
		{"image/webp", anthropic.Base64ImageSourceMediaTypeImageWebP},
		{"application/pdf", anthropic.Base64ImageSourceMediaTypeImagePNG}, // fallback para PNG
	}

	for _, c := range cases {
		t.Run(c.contentType, func(t *testing.T) {
			mt, err := toMediaType(c.contentType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mt != c.expected {
				t.Errorf("expected %q, got %q", c.expected, mt)
			}
		})
	}
}

func TestToMediaType_Unsupported(t *testing.T) {
	unsupported := []string{
		"text/plain",
		"application/json",
		"video/mp4",
		"",
	}

	for _, ct := range unsupported {
		t.Run(ct, func(t *testing.T) {
			_, err := toMediaType(ct)
			if err == nil {
				t.Errorf("expected error for content type %q", ct)
			}
		})
	}
}

// ──────────────────────────────────────────────
// stripMarkdownFences
// ──────────────────────────────────────────────

func TestStripMarkdownFences(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON without fences",
			input:    `{"key":"value"}`,
			expected: `{"key":"value"}`,
		},
		{
			name:     "json fences with language tag",
			input:    "```json\n{\"key\":\"value\"}\n```",
			expected: `{"key":"value"}`,
		},
		{
			name:     "fences without language tag",
			input:    "```\n{\"key\":\"value\"}\n```",
			expected: `{"key":"value"}`,
		},
		{
			name:     "leading and trailing whitespace",
			input:    "  {\"key\":\"value\"}  ",
			expected: `{"key":"value"}`,
		},
		{
			name:     "fences with surrounding whitespace",
			input:    "  ```json\n{\"key\":\"value\"}\n```  ",
			expected: `{"key":"value"}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := stripMarkdownFences(c.input)
			if result != c.expected {
				t.Errorf("expected %q, got %q", c.expected, result)
			}
		})
	}
}

// ──────────────────────────────────────────────
// parseAndValidate
// ──────────────────────────────────────────────

func TestParseAndValidate_Valid(t *testing.T) {
	raw := `{"components":["svc-a","db"],"risks":["no auth","open port"],"recommendations":["add mTLS","close port 22"]}`

	analysis, err := parseAndValidate(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(analysis.Components) != 2 {
		t.Errorf("expected 2 components, got %d", len(analysis.Components))
	}
	if len(analysis.Risks) != 2 {
		t.Errorf("expected 2 risks, got %d", len(analysis.Risks))
	}
	if len(analysis.Recommendations) != 2 {
		t.Errorf("expected 2 recommendations, got %d", len(analysis.Recommendations))
	}
	if analysis.Components[0] != "svc-a" {
		t.Errorf("expected svc-a, got %s", analysis.Components[0])
	}
}

func TestParseAndValidate_WithMarkdownFences(t *testing.T) {
	raw := "```json\n{\"components\":[\"c1\"],\"risks\":[\"r1\"],\"recommendations\":[\"rec1\"]}\n```"

	analysis, err := parseAndValidate(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis == nil {
		t.Fatal("expected non-nil analysis")
	}
	if len(analysis.Components) != 1 || analysis.Components[0] != "c1" {
		t.Errorf("unexpected components: %v", analysis.Components)
	}
}

func TestParseAndValidate_InvalidJSON(t *testing.T) {
	_, err := parseAndValidate("not-json-at-all")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseAndValidate_EmptyComponents(t *testing.T) {
	raw := `{"components":[],"risks":["r1"],"recommendations":["rec1"]}`

	_, err := parseAndValidate(raw)
	if err == nil {
		t.Error("expected error for empty components array")
	}
}

func TestParseAndValidate_EmptyRisks(t *testing.T) {
	raw := `{"components":["c1"],"risks":[],"recommendations":["rec1"]}`

	_, err := parseAndValidate(raw)
	if err == nil {
		t.Error("expected error for empty risks array")
	}
}

func TestParseAndValidate_EmptyRecommendations(t *testing.T) {
	raw := `{"components":["c1"],"risks":["r1"],"recommendations":[]}`

	_, err := parseAndValidate(raw)
	if err == nil {
		t.Error("expected error for empty recommendations array")
	}
}

func TestParseAndValidate_MissingFields(t *testing.T) {
	// JSON sem nenhuma das chaves obrigatórias
	_, err := parseAndValidate(`{}`)
	if err == nil {
		t.Error("expected error for JSON missing all required fields")
	}
}

func TestParseAndValidate_ReturnType(t *testing.T) {
	raw := `{"components":["c1"],"risks":["r1"],"recommendations":["rec1"]}`

	analysis, err := parseAndValidate(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var _ *domain.Analysis = analysis // garante que o retorno é *domain.Analysis
}

// ──────────────────────────────────────────────
// extractText
// ──────────────────────────────────────────────

func TestExtractText_WithTextBlock(t *testing.T) {
	// Constrói anthropic.Message via JSON para ser agnóstico à estrutura interna do SDK.
	var msg anthropic.Message
	raw := `{"content":[{"type":"text","text":"hello world"}]}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	result := extractText(&msg)
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestExtractText_NoTextBlock(t *testing.T) {
	var msg anthropic.Message
	// Bloco de tipo desconhecido, sem campo text
	raw := `{"content":[]}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	result := extractText(&msg)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestExtractText_MultipleBlocks_ReturnsFirst(t *testing.T) {
	var msg anthropic.Message
	raw := `{"content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	result := extractText(&msg)
	if result != "first" {
		t.Errorf("expected 'first', got %q", result)
	}
}
