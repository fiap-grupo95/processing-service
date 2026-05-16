package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/fiap/secure-systems/processing-service/internal/domain"
)

const analysisPrompt = `You are an expert software architect specializing in security analysis of system architectures.

Analyze the provided architecture diagram and identify:
1. All components: services, databases, queues, load balancers, APIs, and infrastructure elements
2. Security risks: vulnerabilities, missing controls, insecure patterns, attack surfaces
3. Technical recommendations: concrete actions to improve security and resilience

Rules:
- Return ONLY a valid JSON object. No markdown, no code blocks, no extra text.
- Arrays must contain at least one item each.
- Be specific and technical in your findings.

Required format:
{"components":["..."],"risks":["..."],"recommendations":["..."]}`

type AnthropicClient struct {
	client    *anthropic.Client
	model     string
	maxTokens int64
}

func NewAnthropicClient(apiKey, model string, maxTokens int64) *AnthropicClient {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicClient{client: client, model: model, maxTokens: maxTokens}
}

func (c *AnthropicClient) Analyze(ctx context.Context, imageData []byte, contentType string) (*domain.Analysis, string, error) {
	mediaType, err := toMediaType(contentType)
	if err != nil {
		return nil, "", err
	}

	b64Image := base64.StdEncoding.EncodeToString(imageData)

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.Model(c.model)),
		MaxTokens: anthropic.F(c.maxTokens),
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64(string(mediaType), b64Image),
				anthropic.NewTextBlock(analysisPrompt),
			),
		}),
	})
	if err != nil {
		return nil, "", fmt.Errorf("anthropic api call: %w", err)
	}

	rawResponse := extractText(msg)
	analysis, err := parseAndValidate(rawResponse)
	if err != nil {
		return nil, rawResponse, fmt.Errorf("%w: %v", domain.ErrLLMGuardrail, err)
	}

	return analysis, rawResponse, nil
}

func toMediaType(contentType string) (anthropic.Base64ImageSourceMediaType, error) {
	switch {
	case strings.HasPrefix(contentType, "image/jpeg"):
		return anthropic.Base64ImageSourceMediaTypeImageJPEG, nil
	case strings.HasPrefix(contentType, "image/png"):
		return anthropic.Base64ImageSourceMediaTypeImagePNG, nil
	case strings.HasPrefix(contentType, "image/gif"):
		return anthropic.Base64ImageSourceMediaTypeImageGIF, nil
	case strings.HasPrefix(contentType, "image/webp"):
		return anthropic.Base64ImageSourceMediaTypeImageWebP, nil
	case strings.HasPrefix(contentType, "application/pdf"):
		// PDF requer pré-conversão para imagem; fallback para PNG para o MVP
		return anthropic.Base64ImageSourceMediaTypeImagePNG, nil
	default:
		return "", fmt.Errorf("unsupported content type for vision: %s", contentType)
	}
}

func extractText(msg *anthropic.Message) string {
	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}

// parseAndValidate faz parse do JSON e valida consistência (guardrail de IA).
func parseAndValidate(raw string) (*domain.Analysis, error) {
	cleaned := stripMarkdownFences(raw)

	var analysis domain.Analysis
	if err := json.Unmarshal([]byte(cleaned), &analysis); err != nil {
		return nil, fmt.Errorf("invalid json from llm: %w", err)
	}
	if len(analysis.Components) == 0 {
		return nil, fmt.Errorf("components array is empty")
	}
	if len(analysis.Risks) == 0 {
		return nil, fmt.Errorf("risks array is empty")
	}
	if len(analysis.Recommendations) == 0 {
		return nil, fmt.Errorf("recommendations array is empty")
	}
	return &analysis, nil
}

// stripMarkdownFences remove ```json ... ``` caso o modelo os adicione.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
