package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DynamoDBEndpoint string
	DynamoDBRegion   string
	DynamoDBTable    string
	MinioEndpoint    string
	MinioAccessKey   string
	MinioSecretKey   string
	MinioBucket      string
	MinioUseSSL      bool
	RabbitMQURL      string
	ProcessQueue     string
	ReportQueue      string
	ProcessingTopic  string
	LLMProvider      string
	LLMAPIKey        string
	LLMBaseURL       string
	LLMModel         string
	LLMMaxTokens     int64
}

func Load() (*Config, error) {
	ssl, _ := strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))
	maxTokens, _ := strconv.ParseInt(getEnv("LLM_MAX_TOKENS", "4096"), 10, 64)
	if maxTokens <= 0 || maxTokens > 8192 {
		return nil, fmt.Errorf("LLM_MAX_TOKENS must be between 1 and 8192")
	}

	return &Config{
		DynamoDBEndpoint: getEnv("DYNAMODB_ENDPOINT", ""),
		DynamoDBRegion:   getEnv("DYNAMODB_REGION", "us-east-1"),
		DynamoDBTable:    getEnv("DYNAMODB_TABLE", "processing_jobs"),
		MinioEndpoint:    requireEnv("MINIO_ENDPOINT"),
		MinioAccessKey:   requireEnv("MINIO_ACCESS_KEY"),
		MinioSecretKey:   requireEnv("MINIO_SECRET_KEY"),
		MinioBucket:      getEnv("MINIO_BUCKET", "diagrams"),
		MinioUseSSL:      ssl,
		RabbitMQURL:      requireEnv("RABBITMQ_URL"),
		ProcessQueue:     getEnv("PROCESS_QUEUE", "process.queue"),
		ReportQueue:      getEnv("REPORT_QUEUE", "report.queue"),
		ProcessingTopic:  getEnv("PROCESSING_TOPIC", "processing.topic"),
		LLMProvider:      getEnv("LLM_PROVIDER", "anthropic"),
		LLMAPIKey:        requireEnv("LLM_API_KEY"),
		LLMBaseURL:       getEnv("LLM_BASE_URL", ""),
		LLMModel:         getEnv("LLM_MODEL", "claude-sonnet-4-6"),
		LLMMaxTokens:     maxTokens,
	}, nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %s is not set", key))
	}
	return v
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
