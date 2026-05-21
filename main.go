package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/fiap/secure-systems/processing-service/internal/ai"
	"github.com/fiap/secure-systems/processing-service/internal/config"
	"github.com/fiap/secure-systems/processing-service/internal/consumer"
	"github.com/fiap/secure-systems/processing-service/internal/queue"
	"github.com/fiap/secure-systems/processing-service/internal/repository"
	"github.com/fiap/secure-systems/processing-service/internal/storage"
	"github.com/fiap/secure-systems/processing-service/internal/usecase"
	"github.com/newrelic/go-agent/v3/newrelic"
	"go.uber.org/zap"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config load failed", zap.Error(err))
	}

	// ─── New Relic ────────────────────────────────────────────────────────────
	nrApp, err := newrelic.NewApplication(newrelic.ConfigFromEnvironment())
	if err != nil {
		log.Warn("new relic not configured", zap.Error(err))
		nrApp, _ = newrelic.NewApplication(newrelic.ConfigEnabled(false))
	}

	// ─── DynamoDB ─────────────────────────────────────────────────────────────
	awsAccessKey := envOrDefault("AWS_ACCESS_KEY_ID", "fakekey")
	awsSecretKey := envOrDefault("AWS_SECRET_ACCESS_KEY", "fakesecret")

	dynaCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.DynamoDBRegion),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, ""),
		),
	)
	if err != nil {
		log.Fatal("aws config failed", zap.Error(err))
	}

	dynamoOpts := []func(*dynamodb.Options){}
	if cfg.DynamoDBEndpoint != "" {
		endpoint := cfg.DynamoDBEndpoint
		dynamoOpts = append(dynamoOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	dynamoClient := dynamodb.NewFromConfig(dynaCfg, dynamoOpts...)

	jobRepo := repository.NewJobRepository(dynamoClient, cfg.DynamoDBTable)
	if err := jobRepo.EnsureTable(context.Background()); err != nil {
		log.Fatal("dynamodb table setup failed", zap.Error(err))
	}

	// ─── MinIO ────────────────────────────────────────────────────────────────
	minioDownloader, err := storage.NewMinIODownloader(
		cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, cfg.MinioUseSSL,
	)
	if err != nil {
		log.Fatal("minio init failed", zap.Error(err))
	}

	// ─── LLM Client ───────────────────────────────────────────────────────────
	var llmClient usecase.LLMClient
	switch cfg.LLMProvider {
	case "openai":
		llmClient = ai.NewOpenAIClient(cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMMaxTokens)
	case "gemini":
		llmClient = ai.NewGeminiClient(cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMMaxTokens)
	default:
		llmClient = ai.NewAnthropicClient(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel, cfg.LLMMaxTokens)
	}

	// ─── RabbitMQ ─────────────────────────────────────────────────────────────
	rmq, err := queue.NewRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Fatal("rabbitmq connect failed", zap.Error(err))
	}
	defer rmq.Close()

	for _, q := range []string{cfg.ProcessQueue, cfg.ReportQueue} {
		if err := rmq.DeclareQueue(q); err != nil {
			log.Fatal("declare queue failed", zap.String("queue", q), zap.Error(err))
		}
	}
	if err := rmq.DeclareExchange(cfg.ProcessingTopic); err != nil {
		log.Fatal("declare processing exchange failed", zap.Error(err))
	}

	deliveries, err := rmq.Consume(cfg.ProcessQueue)
	if err != nil {
		log.Fatal("consume process queue failed", zap.Error(err))
	}

	// ─── Caso de Uso ──────────────────────────────────────────────────────────
	processUC := usecase.NewProcessDiagramUseCase(
		jobRepo, minioDownloader, llmClient, rmq,
		cfg.ProcessingTopic, cfg.ReportQueue, log,
	)

	// ─── Consumer (bloqueia até SIGINT/SIGTERM) ───────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("processing-service started")
	consumer.NewProcessQueueConsumer(processUC, nrApp, log).Run(ctx, deliveries)
	log.Info("processing-service stopped")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
