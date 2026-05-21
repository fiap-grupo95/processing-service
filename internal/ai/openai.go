package ai

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fiap/secure-systems/processing-service/internal/domain"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type OpenAIClient struct {
	client    openai.Client
	model     string
	maxTokens int64
}

func NewOpenAIClient(apiKey, model string, maxTokens int64) *OpenAIClient {
	httpCli := &http.Client{Timeout: 120 * time.Second}
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithHTTPClient(httpCli),
	)
	return &OpenAIClient{client: client, model: model, maxTokens: maxTokens}
}

func (c *OpenAIClient) Analyze(ctx context.Context, imageData []byte, contentType string) (*domain.Analysis, string, error) {
	mimeType, err := toImageMIMEString(contentType)
	if err != nil {
		return nil, "", err
	}

	dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(imageData)

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(c.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
					URL: dataURL,
				}),
				openai.TextContentPart(analysisPrompt),
			}),
		},
		MaxCompletionTokens: openai.Int(c.maxTokens),
	})
	if err != nil {
		return nil, "", fmt.Errorf("openai api call: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, "", fmt.Errorf("openai returned no choices")
	}

	rawResponse := resp.Choices[0].Message.Content
	analysis, err := parseAndValidate(rawResponse)
	if err != nil {
		return nil, rawResponse, fmt.Errorf("%w: %v", domain.ErrLLMGuardrail, err)
	}

	return analysis, rawResponse, nil
}

func toImageMIMEString(contentType string) (string, error) {
	switch {
	case strings.HasPrefix(contentType, "image/jpeg"):
		return "image/jpeg", nil
	case strings.HasPrefix(contentType, "image/png"):
		return "image/png", nil
	case strings.HasPrefix(contentType, "image/gif"):
		return "image/gif", nil
	case strings.HasPrefix(contentType, "image/webp"):
		return "image/webp", nil
	default:
		return "", fmt.Errorf("unsupported content type for openai vision: %s", contentType)
	}
}
