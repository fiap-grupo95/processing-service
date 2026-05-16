package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/fiap/secure-systems/processing-service/internal/domain"
)

type JobRepository struct {
	client    *dynamodb.Client
	tableName string
}

func NewJobRepository(client *dynamodb.Client, tableName string) *JobRepository {
	return &JobRepository{client: client, tableName: tableName}
}

// EnsureTable cria a tabela se ainda não existir (idempotente).
func (r *JobRepository) EnsureTable(ctx context.Context) error {
	_, err := r.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(r.tableName),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("process_id"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("process_id"), AttributeType: types.ScalarAttributeTypeS},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		// ResourceInUseException significa que a tabela já existe — ok
		var resInUse *types.ResourceInUseException
		if !errors.As(err, &resInUse) {
			return fmt.Errorf("create dynamodb table: %w", err)
		}
	}
	return nil
}

func (r *JobRepository) Save(ctx context.Context, job *domain.ProcessingJob) error {
	_, err := r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item: map[string]types.AttributeValue{
			"process_id": strAttr(job.ProcessID),
			"status":     strAttr(string(job.Status)),
			"started_at": strAttr(job.StartedAt.Format(time.RFC3339)),
		},
	})
	return err
}

func (r *JobRepository) UpdateCompleted(ctx context.Context, processID, llmResponse string, analysis *domain.Analysis) error {
	analysisJSON, err := json.Marshal(analysis)
	if err != nil {
		return fmt.Errorf("marshal analysis: %w", err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key:       map[string]types.AttributeValue{"process_id": strAttr(processID)},
		UpdateExpression: aws.String(
			"SET #s = :status, llm_response = :llm, analysis = :analysis, completed_at = :completed",
		),
		ExpressionAttributeNames: map[string]string{"#s": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":    strAttr(string(domain.JobStatusCompleted)),
			":llm":       strAttr(llmResponse),
			":analysis":  strAttr(string(analysisJSON)),
			":completed": strAttr(time.Now().UTC().Format(time.RFC3339)),
		},
	})
	return err
}

func (r *JobRepository) UpdateError(ctx context.Context, processID, errMsg string) error {
	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key:       map[string]types.AttributeValue{"process_id": strAttr(processID)},
		UpdateExpression: aws.String(
			"SET #s = :status, error_msg = :err, completed_at = :completed",
		),
		ExpressionAttributeNames: map[string]string{"#s": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":    strAttr(string(domain.JobStatusError)),
			":err":       strAttr(errMsg),
			":completed": strAttr(time.Now().UTC().Format(time.RFC3339)),
		},
	})
	return err
}

func strAttr(v string) types.AttributeValue {
	return &types.AttributeValueMemberS{Value: v}
}
