#!/usr/bin/env bash
# =============================================================================
# Processing Service — Documentação
# Este serviço NÃO expõe endpoints HTTP. É um consumer RabbitMQ puro.
# Use os comandos abaixo para monitorar e testar via RabbitMQ Management API.
#
# Management UI: http://localhost:15672
# Usuário: guest | Senha: definida em RABBITMQ_PASSWORD no .env
# =============================================================================

RABBITMQ_URL="http://localhost:15672"
RABBITMQ_USER="guest"
RABBITMQ_PASS="${RABBITMQ_PASSWORD:-dev_rabbitmq_pass}"

# ─── Estado das Filas ─────────────────────────────────────────────────────────

# process.queue: mensagens aguardando processamento pela IA
curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  "${RABBITMQ_URL}/api/queues/%2F/process.queue" | \
  python3 -m json.tool 2>/dev/null || \
  curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  "${RABBITMQ_URL}/api/queues/%2F/process.queue"

# report.queue: análises aguardando geração de relatório
curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  "${RABBITMQ_URL}/api/queues/%2F/report.queue" | \
  python3 -m json.tool 2>/dev/null || \
  curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  "${RABBITMQ_URL}/api/queues/%2F/report.queue"

# ─── Exchanges (tópicos de feedback) ─────────────────────────────────────────

# Lista todos os exchanges
curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  "${RABBITMQ_URL}/api/exchanges/%2F" | python3 -m json.tool

# Detalhes do exchange processing.topic (publicado pelo processing-service)
curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  "${RABBITMQ_URL}/api/exchanges/%2F/processing.topic"

# Detalhes do exchange report.topic (publicado pelo report-service)
curl -s -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  "${RABBITMQ_URL}/api/exchanges/%2F/report.topic"

# ─── Injetar Mensagem de Teste na process.queue ───────────────────────────────
# Útil para testar o processing-service sem subir o fluxo completo.
# Substitua process_id e s3_key por valores reais (o arquivo deve existir no MinIO).
#
PROCESS_ID="${PROCESS_ID:-00000000-0000-0000-0000-000000000001}"
S3_KEY="diagrams/${PROCESS_ID}"

curl -i -X POST \
  -H "Content-Type: application/json" \
  -u "${RABBITMQ_USER}:${RABBITMQ_PASS}" \
  -d "{
    \"properties\": {
      \"delivery_mode\": 2,
      \"content_type\": \"application/json\"
    },
    \"routing_key\": \"process.queue\",
    \"payload\": \"{\\\"process_id\\\":\\\"${PROCESS_ID}\\\",\\\"s3_key\\\":\\\"${S3_KEY}\\\",\\\"content_type\\\":\\\"image/png\\\"}\",
    \"payload_encoding\": \"string\"
  }" \
  "${RABBITMQ_URL}/api/exchanges/%2F/amq.default/publish"

# ─── Verificação no DynamoDB Local ───────────────────────────────────────────
# Lista todos os jobs de processamento
curl -s -X POST \
  -H "Content-Type: application/x-amz-json-1.0" \
  -H "X-Amz-Target: DynamoDB_20120810.Scan" \
  -H "Authorization: AWS4-HMAC-SHA256 Credential=fakekey/20260516/us-east-1/dynamodb/aws4_request" \
  --data '{"TableName": "processing_jobs"}' \
  http://localhost:8000

# Alternativa via AWS CLI (se instalado):
#   AWS_ACCESS_KEY_ID=fakekey AWS_SECRET_ACCESS_KEY=fakesecret \
#   aws dynamodb scan \
#     --table-name processing_jobs \
#     --endpoint-url http://localhost:8000 \
#     --region us-east-1

# ─── Logs do Container ────────────────────────────────────────────────────────
# docker logs -f hacka-processing-service-1
