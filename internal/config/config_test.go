package config

import (
	"os"
	"testing"
)

// requiredEnvs retorna as variáveis de ambiente obrigatórias com valores válidos para testes.
func requiredEnvs() map[string]string {
	return map[string]string{
		"MINIO_ENDPOINT":   "localhost:9000",
		"MINIO_ACCESS_KEY": "access-key",
		"MINIO_SECRET_KEY": "secret-key",
		"RABBITMQ_URL":     "amqp://guest:guest@localhost:5672/",
		"LLM_API_KEY":      "sk-test-key",
	}
}

func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	setEnvs(t, requiredEnvs())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name     string
		got      string
		expected string
	}{
		{"DynamoDBRegion", cfg.DynamoDBRegion, "us-east-1"},
		{"DynamoDBTable", cfg.DynamoDBTable, "processing_jobs"},
		{"MinioBucket", cfg.MinioBucket, "diagrams"},
		{"ProcessQueue", cfg.ProcessQueue, "process.queue"},
		{"ReportQueue", cfg.ReportQueue, "report.queue"},
		{"ProcessingTopic", cfg.ProcessingTopic, "processing.topic"},
		{"LLMModel", cfg.LLMModel, "claude-sonnet-4-6"},
		{"DynamoDBEndpoint", cfg.DynamoDBEndpoint, ""},
		{"LLMBaseURL", cfg.LLMBaseURL, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.expected {
				t.Errorf("expected %q, got %q", c.expected, c.got)
			}
		})
	}

	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("expected LLMMaxTokens=4096, got %d", cfg.LLMMaxTokens)
	}
	if cfg.MinioUseSSL != false {
		t.Error("expected MinioUseSSL=false by default")
	}
}

func TestLoad_RequiredEnvValues(t *testing.T) {
	envs := requiredEnvs()
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MinioEndpoint != "localhost:9000" {
		t.Errorf("expected localhost:9000, got %s", cfg.MinioEndpoint)
	}
	if cfg.MinioAccessKey != "access-key" {
		t.Errorf("expected access-key, got %s", cfg.MinioAccessKey)
	}
	if cfg.MinioSecretKey != "secret-key" {
		t.Errorf("expected secret-key, got %s", cfg.MinioSecretKey)
	}
	if cfg.RabbitMQURL != "amqp://guest:guest@localhost:5672/" {
		t.Errorf("unexpected RabbitMQURL: %s", cfg.RabbitMQURL)
	}
	if cfg.LLMAPIKey != "sk-test-key" {
		t.Errorf("expected sk-test-key, got %s", cfg.LLMAPIKey)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	envs := requiredEnvs()
	envs["DYNAMODB_ENDPOINT"] = "http://localhost:8000"
	envs["DYNAMODB_REGION"] = "eu-west-1"
	envs["DYNAMODB_TABLE"] = "custom_table"
	envs["MINIO_BUCKET"] = "custom-bucket"
	envs["MINIO_USE_SSL"] = "true"
	envs["PROCESS_QUEUE"] = "custom.queue"
	envs["REPORT_QUEUE"] = "custom.report"
	envs["PROCESSING_TOPIC"] = "custom.topic"
	envs["LLM_BASE_URL"] = "https://custom.api.com"
	envs["LLM_MODEL"] = "custom-model"
	envs["LLM_MAX_TOKENS"] = "2048"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DynamoDBEndpoint != "http://localhost:8000" {
		t.Errorf("expected http://localhost:8000, got %s", cfg.DynamoDBEndpoint)
	}
	if cfg.DynamoDBRegion != "eu-west-1" {
		t.Errorf("expected eu-west-1, got %s", cfg.DynamoDBRegion)
	}
	if cfg.DynamoDBTable != "custom_table" {
		t.Errorf("expected custom_table, got %s", cfg.DynamoDBTable)
	}
	if cfg.MinioBucket != "custom-bucket" {
		t.Errorf("expected custom-bucket, got %s", cfg.MinioBucket)
	}
	if cfg.MinioUseSSL != true {
		t.Error("expected MinioUseSSL=true")
	}
	if cfg.ProcessQueue != "custom.queue" {
		t.Errorf("expected custom.queue, got %s", cfg.ProcessQueue)
	}
	if cfg.ReportQueue != "custom.report" {
		t.Errorf("expected custom.report, got %s", cfg.ReportQueue)
	}
	if cfg.ProcessingTopic != "custom.topic" {
		t.Errorf("expected custom.topic, got %s", cfg.ProcessingTopic)
	}
	if cfg.LLMBaseURL != "https://custom.api.com" {
		t.Errorf("expected https://custom.api.com, got %s", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "custom-model" {
		t.Errorf("expected custom-model, got %s", cfg.LLMModel)
	}
	if cfg.LLMMaxTokens != 2048 {
		t.Errorf("expected 2048, got %d", cfg.LLMMaxTokens)
	}
}

func TestLoad_InvalidMaxTokens_Zero(t *testing.T) {
	setEnvs(t, requiredEnvs())
	t.Setenv("LLM_MAX_TOKENS", "0")

	_, err := Load()
	if err == nil {
		t.Error("expected error for LLM_MAX_TOKENS=0")
	}
}

func TestLoad_InvalidMaxTokens_Negative(t *testing.T) {
	setEnvs(t, requiredEnvs())
	t.Setenv("LLM_MAX_TOKENS", "-1")

	_, err := Load()
	if err == nil {
		t.Error("expected error for LLM_MAX_TOKENS=-1")
	}
}

func TestLoad_InvalidMaxTokens_TooLarge(t *testing.T) {
	setEnvs(t, requiredEnvs())
	t.Setenv("LLM_MAX_TOKENS", "9000")

	_, err := Load()
	if err == nil {
		t.Error("expected error for LLM_MAX_TOKENS=9000")
	}
}

func TestLoad_MaxTokens_Boundary(t *testing.T) {
	setEnvs(t, requiredEnvs())
	t.Setenv("LLM_MAX_TOKENS", "8192")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error for LLM_MAX_TOKENS=8192, got: %v", err)
	}
	if cfg.LLMMaxTokens != 8192 {
		t.Errorf("expected 8192, got %d", cfg.LLMMaxTokens)
	}
}

func TestLoad_MaxTokens_NonNumeric(t *testing.T) {
	setEnvs(t, requiredEnvs())
	t.Setenv("LLM_MAX_TOKENS", "not-a-number")

	// ParseInt falha silenciosamente, resultando em 0, então o Load deve retornar erro
	_, err := Load()
	if err == nil {
		t.Error("expected error for non-numeric LLM_MAX_TOKENS")
	}
}

func TestRequireEnv_Panics_WhenMissing(t *testing.T) {
	const key = "TEST_REQUIRED_VAR_MISSING_XYZ"
	os.Unsetenv(key)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing required env var")
		}
	}()

	requireEnv(key)
}

func TestRequireEnv_ReturnsValue(t *testing.T) {
	const key = "TEST_REQUIRED_VAR_SET_XYZ"
	t.Setenv(key, "myvalue")

	val := requireEnv(key)
	if val != "myvalue" {
		t.Errorf("expected myvalue, got %s", val)
	}
}

func TestGetEnv_ReturnsDefault_WhenNotSet(t *testing.T) {
	const key = "TEST_GETENV_UNSET_XYZ"
	os.Unsetenv(key)

	val := getEnv(key, "fallback")
	if val != "fallback" {
		t.Errorf("expected fallback, got %s", val)
	}
}

func TestGetEnv_ReturnsEnvValue_WhenSet(t *testing.T) {
	const key = "TEST_GETENV_SET_XYZ"
	t.Setenv(key, "override")

	val := getEnv(key, "fallback")
	if val != "override" {
		t.Errorf("expected override, got %s", val)
	}
}
