package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fiap/secure-systems/processing-service/internal/domain"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GeminiClient struct {
	apiKey    string
	model     string
	maxTokens int32
}

func NewGeminiClient(apiKey, model string, maxTokens int64) *GeminiClient {
	return &GeminiClient{
		apiKey:    apiKey,
		model:     model,
		maxTokens: int32(maxTokens),
	}
}

func (c *GeminiClient) Analyze(ctx context.Context, imageData []byte, contentType string) (*domain.Analysis, string, error) {
	mimeType, format, err := toGeminiMIME(contentType)
	if err != nil {
		return nil, "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(c.apiKey))
	if err != nil {
		return nil, "", fmt.Errorf("gemini client init: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(c.model)
	maxTok := c.maxTokens
	model.GenerationConfig.MaxOutputTokens = &maxTok
	_ = mimeType

	resp, err := model.GenerateContent(ctx,
		genai.ImageData(format, imageData),
		genai.Text(analysisPrompt),
	)
	if err != nil {
		return nil, "", fmt.Errorf("gemini api call: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, "", fmt.Errorf("gemini returned no content")
	}

	rawResponse := extractGeminiText(resp)
	analysis, err := parseAndValidate(rawResponse)
	if err != nil {
		return nil, rawResponse, fmt.Errorf("%w: %v", domain.ErrLLMGuardrail, err)
	}

	return analysis, rawResponse, nil
}

func extractGeminiText(resp *genai.GenerateContentResponse) string {
	var sb strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if t, ok := part.(genai.Text); ok {
				sb.WriteString(string(t))
			}
		}
	}
	return sb.String()
}

// toGeminiMIME retorna (mimeType completo, formato sem prefixo) validados.
func toGeminiMIME(contentType string) (string, string, error) {
	switch {
	case strings.HasPrefix(contentType, "image/jpeg"):
		return "image/jpeg", "jpeg", nil
	case strings.HasPrefix(contentType, "image/png"):
		return "image/png", "png", nil
	case strings.HasPrefix(contentType, "image/gif"):
		return "image/gif", "gif", nil
	case strings.HasPrefix(contentType, "image/webp"):
		return "image/webp", "webp", nil
	default:
		return "", "", fmt.Errorf("unsupported content type for gemini vision: %s", contentType)
	}
}
