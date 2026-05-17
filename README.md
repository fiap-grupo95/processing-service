# Processing Service

Worker de análise de diagramas por Inteligência Artificial. Consome tarefas de uma fila RabbitMQ, chama o modelo de visão da Anthropic (Claude) e persiste o histórico de execuções no DynamoDB.

---

## Descrição do Problema

A análise de diagramas de arquitetura por IA envolve operações custosas e de longa duração (chamada a API externa, processamento de imagem). Executar isso de forma síncrona na requisição HTTP degradaria a experiência do usuário e tornaria o sistema frágil a timeouts.

**Desafios específicos endereçados:**

- Processar diagramas de forma assíncrona sem bloquear o fluxo de upload
- Garantir que falhas na IA sejam capturadas e o orquestrador seja notificado
- Validar a resposta da IA antes de persistir (guardrail) para evitar relatórios incompletos ou incoerentes
- Manter histórico auditável de cada execução (modelo, prompt, resposta, timestamps)
- Notificar o orquestrador sobre mudanças de estado via eventos desacoplados

---

## Arquitetura Proposta

```mermaid
graph TD
    PQ["RabbitMQ: process.queue"]

    subgraph PS["Processing Service (worker)"]
        CON["consumer/process_queue.go"]
        UC["usecase/process_diagram.go"]
        DY["repository/job_dynamo.go\n(DynamoDB Local)"]
        MN["storage/minio.go\n(download)"]
        AI["ai/anthropic.go\n(Anthropic API)\nguardrail: parse + valida JSON"]
        Q["queue/rabbitmq.go"]
        CON --> UC
        UC --> DY
        UC --> MN
        UC --> AI
        UC --> Q
    end

    PT["processing.topic (fanout)"]
    RQ["report.queue (direct)"]

    PQ -->|consume| CON
    Q --> PT
    Q --> RQ
```

### Guardrail de IA

A resposta do modelo é validada antes de qualquer persistência:

```mermaid
flowchart TD
    R["Resposta LLM (texto)"]
    SM["Strip de markdown fences"]
    JU["json.Unmarshal"]
    C{"components\nnão vazio?"}
    RK{"risks\nnão vazio?"}
    RC{"recommendations\nnão vazio?"}
    OK["Persiste no DynamoDB\n+ publica em report.queue"]
    ERR["UpdateError no DynamoDB\n+ publica processing_error"]

    R --> SM --> JU --> C
    C -->|sim| RK
    C -->|não| ERR
    RK -->|sim| RC
    RK -->|não| ERR
    RC -->|sim| OK
    RC -->|não| ERR
```

### Camadas internas (Clean Architecture)

```
internal/
├── domain/
│   └── job.go            ← ProcessingJob, Analysis, ErrLLMGuardrail
├── usecase/
│   ├── ports.go          ← JobRepository, DiagramDownloader, LLMClient, EventPublisher
│   └── process_diagram.go← orquestra todo o fluxo de análise
├── repository/
│   └── job_dynamo.go     ← EnsureTable (idempotente), Save, UpdateCompleted, UpdateError
├── storage/
│   └── minio.go          ← GetObject + Stat (recupera Content-Type)
├── ai/
│   └── anthropic.go      ← Anthropic SDK, mediaType mapping, guardrail de validação
├── queue/
│   └── rabbitmq.go       ← PublishToQueue (default exchange) + PublishToExchange (fanout)
└── consumer/
    └── process_queue.go  ← loop de consume com New Relic transaction por mensagem
```

### Esquema DynamoDB (`processing_jobs`)

| Campo | Tipo | Descrição |
|---|---|---|
| `process_id` | String (PK) | UUID do processo |
| `status` | String | `PROCESSING` \| `COMPLETED` \| `ERROR` |
| `llm_response` | String | Resposta bruta do modelo |
| `analysis` | String (JSON) | `{components, risks, recommendations}` |
| `started_at` | String (ISO 8601) | Início do processamento |
| `completed_at` | String (ISO 8601) | Fim do processamento |
| `error_msg` | String | Mensagem de erro (se aplicável) |

---

## Fluxo da Solução

```mermaid
flowchart TD
    MSG["Mensagem: process.queue\n{process_id, s3_key, content_type}"]
    D1["DynamoDB.Save\nstatus = PROCESSING"]
    D2["Publica processing.topic\n{event: processing_started}\n→ upload-orchestrator: EM_PROCESSAMENTO"]
    D3["MinIO.GetObject(s3_key)\nDownload da imagem"]
    D4["Anthropic.Messages.New\nbase64 + prompt de análise de segurança"]
    GR["parseAndValidate\nGuardrail JSON"]
    OK{válido?}
    D5["DynamoDB.UpdateCompleted\nanalysis + llm_response + completed_at"]
    D6["Publica report.queue\n{process_id, analysis, raw_response}"]
    ACK["Ack()"]
    ERR1["DynamoDB.UpdateError\nerror_msg + completed_at"]
    ERR2["Publica processing.topic\n{event: processing_error}"]
    NACK["Nack(false, false)\nsem requeue"]

    MSG --> D1 --> D2 --> D3 --> D4 --> GR --> OK
    OK -->|sim| D5 --> D6 --> ACK
    OK -->|não| ERR1 --> ERR2 --> NACK
```

---

## Instruções de Execução

### Variáveis de ambiente

| Variável | Obrigatório | Padrão | Descrição |
|---|---|---|---|
| `LLM_API_KEY` | **Sim** | — | Chave da API Anthropic (`sk-ant-...`) |
| `RABBITMQ_URL` | Sim | — | Ex: `amqp://guest:pass@rabbitmq:5672/` |
| `MINIO_ENDPOINT` | Sim | — | Ex: `minio:9000` |
| `MINIO_ACCESS_KEY` | Sim | — | Chave de acesso MinIO |
| `MINIO_SECRET_KEY` | Sim | — | Chave secreta MinIO |
| `LLM_MODEL` | Não | `claude-sonnet-4-6` | Modelo Anthropic a usar |
| `LLM_MAX_TOKENS` | Não | `4096` | Tokens máximos na resposta |
| `DYNAMODB_ENDPOINT` | Não | — | URL do DynamoDB Local (ex: `http://dynamodb-local:8000`) |
| `DYNAMODB_REGION` | Não | `us-east-1` | Região AWS |
| `DYNAMODB_TABLE` | Não | `processing_jobs` | Nome da tabela |
| `PROCESS_QUEUE` | Não | `process.queue` | Fila de entrada |
| `REPORT_QUEUE` | Não | `report.queue` | Fila de saída |
| `PROCESSING_TOPIC` | Não | `processing.topic` | Exchange de eventos de status |
| `AWS_ACCESS_KEY_ID` | Não | `fakekey` | Aceita qualquer valor para DynamoDB Local |
| `AWS_SECRET_ACCESS_KEY` | Não | `fakesecret` | Aceita qualquer valor para DynamoDB Local |
| `NEW_RELIC_LICENSE_KEY` | Não | — | Chave New Relic |

> **Importante:** `LLM_API_KEY` é a única variável sem valor padrão funcional. O serviço não inicia sem ela.

### Executar com Docker Compose (recomendado)

```bash
# A partir da raiz do projeto
# Certifique-se de que LLM_API_KEY está no .env
docker compose up --build -d processing-service

# Acompanhar logs em tempo real
docker logs -f hacka-processing-service-1
```

### Executar localmente (desenvolvimento)

```bash
cd processing-service
go mod download

export LLM_API_KEY="sk-ant-..."
export RABBITMQ_URL="amqp://guest:dev_rabbitmq_pass@localhost:5672/"
export MINIO_ENDPOINT="localhost:9000"
export MINIO_ACCESS_KEY="minioadmin"
export MINIO_SECRET_KEY="dev_minio_pass"
export DYNAMODB_ENDPOINT="http://localhost:8000"
export AWS_ACCESS_KEY_ID="fakekey"
export AWS_SECRET_ACCESS_KEY="fakesecret"

go run main.go
```

### Inspecionar DynamoDB Local

```bash
# Via AWS CLI (se instalado)
AWS_ACCESS_KEY_ID=fakekey AWS_SECRET_ACCESS_KEY=fakesecret \
aws dynamodb scan \
  --table-name processing_jobs \
  --endpoint-url http://localhost:8000 \
  --region us-east-1 \
  --query 'Items[*].{ID:process_id.S, Status:status.S, Started:started_at.S}'
```

### Injetar mensagem de teste

```bash
# Via RabbitMQ Management API (o arquivo deve existir no MinIO)
curl -s -X POST \
  -H "Content-Type: application/json" \
  -u guest:dev_rabbitmq_pass \
  -d '{
    "properties": {"delivery_mode": 2},
    "routing_key": "process.queue",
    "payload": "{\"process_id\":\"SEU-UUID\",\"s3_key\":\"diagrams/SEU-UUID\",\"content_type\":\"image/png\"}",
    "payload_encoding": "string"
  }' \
  http://localhost:15672/api/exchanges/%2F/amq.default/publish
```

---

## Segurança

### Requisitos básicos adotados

| Controle | Implementação |
|---|---|
| Sem endpoints HTTP expostos | Serviço é consumer puro — sem superfície de ataque HTTP |
| Credenciais via env | `LLM_API_KEY`, credenciais MinIO e RabbitMQ nunca no código |
| Chave DynamoDB segura | `UpdateExpression` com `ExpressionAttributeNames` — sem injeção em nomes de campo reservados |
| Nack sem requeue | Falhas de payload inválido ou guardrail descartam a mensagem definitivamente |
| Escopo do modelo fixado | Modelo Anthropic definido por `LLM_MODEL` (env) — não selecionável pelo usuário |

### Validação de entradas não confiáveis

- **Mensagens RabbitMQ:** `json.Unmarshal` + validação de `ProcessID` e `S3Key` não vazios antes de qualquer operação; `Nack(false, false)` em falha.
- **`processId`:** nenhum componente de entrada do usuário é passado para o DynamoDB sem validação — o `process_id` vem exclusivamente da mensagem publicada pelo upload-orchestrator (gerado internamente como UUID).
- **Chave MinIO (`s3_key`):** recebida da mensagem, não do usuário; composta por `diagrams/{uuid}` gerado pelo servidor no upload-orchestrator.
- **Resposta da IA:** tratada como entrada não confiável e validada pelo guardrail antes de qualquer persistência.

### Uso controlado de modelos de IA — escopo e previsibilidade

O modelo é instruído com um prompt de sistema fixo que restringe o escopo exclusivamente à análise de arquitetura de software:

```
Analyze the provided architecture diagram and identify:
1. All components: services, databases, queues, load balancers...
2. Security risks: vulnerabilities, missing controls, insecure patterns...
3. Technical recommendations: concrete actions to improve security...
Rules:
- Return ONLY a valid JSON object. No markdown, no code blocks, no extra text.
- Arrays must contain at least one item each.
```

Mecanismos de previsibilidade:
- **Modelo fixado:** `LLM_MODEL` via env — sem seleção dinâmica
- **MaxTokens limitado:** configurável entre 1 e 8192 tokens
- **Formato de saída especificado:** JSON com schema pré-definido
- **Instrução de restrição:** "Return ONLY" — minimiza saída fora do formato esperado

### Tratamento seguro de falhas da IA

O guardrail (`parseAndValidate` em `internal/ai/anthropic.go`) executa em toda resposta antes de qualquer escrita:

| Etapa | O que verifica | Ação em falha |
|---|---|---|
| Strip de markdown | Remove ` ```json ``` ` gerados pelo modelo | Continua com conteúdo limpo |
| `json.Unmarshal` | JSON válido | `ErrLLMGuardrail` → failJob |
| `components` não vazio | Pelo menos 1 componente identificado | `ErrLLMGuardrail` → failJob |
| `risks` não vazio | Pelo menos 1 risco identificado | `ErrLLMGuardrail` → failJob |
| `recommendations` não vazio | Pelo menos 1 recomendação | `ErrLLMGuardrail` → failJob |

Em `failJob`:
1. `DynamoDB.UpdateError` persiste o estado de erro e a mensagem descritiva
2. Publica `processing_error` no `processing.topic` — orquestrador atualiza PostgreSQL para `ERRO`
3. `Nack(false, false)` — mensagem descartada sem requeue

### Comunicação entre serviços

- Publicação no `processing.topic` (fanout) com `delivery_mode: Persistent` — garante entrega mesmo com restart
- Publicação no `report.queue` com `delivery_mode: Persistent` e `Content-Type: application/json`
- Sem chamadas HTTP diretas a outros microsserviços — toda comunicação via RabbitMQ
- Credenciais AWS (fakekey/fakesecret) usadas apenas para DynamoDB Local — em produção, usar IAM roles

### Principais riscos e limitações

| Risco | Severidade | Mitigação atual | Recomendação para produção |
|---|---|---|---|
| LLM pode alucinar findings | Alta | Guardrail de formato e campos obrigatórios | Validação semântica adicional (ex: verificar se componentes são reais) |
| DynamoDB Local não persiste entre restarts | Média (dev) | Flags `-inMemory` | Usar DynamoDB AWS com Point-in-Time Recovery |
| Sem retry em falha de chamada à API Anthropic | Média | Erro vai para ERRO imediatamente | Implementar retry com backoff exponencial e dead letter queue |
| Imagem enviada em base64 (sem verificação de conteúdo) | Baixa | MIME já validado no gateway | Adicionar detecção de conteúdo inadequado antes da chamada |
| `raw_response` armazenado sem sanitização | Baixa | Campo não é renderizado em HTML | Sanitizar se futuramente renderizado em interface web |

---

### Build

```bash
cd processing-service
go build ./...
go vet ./...
```
