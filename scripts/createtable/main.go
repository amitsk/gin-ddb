// Command createtable creates the SkiLifts table with keys and the
// SkiLiftsByRiders GSI per spec §7. Idempotent: ResourceInUse is success.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"

	"gin-ddb/internal/config"
	dbpkg "gin-ddb/internal/db"
)

// buildInput renders the CreateTable request: PK Lift (S) + SK Metadata
// (S), GSI SkiLiftsByRiders on Lift (S) + TotalUniqueLiftRiders (N) with
// ALL projection. Billing is PAY_PER_REQUEST (no capacity to tune for
// local/dev; production override recorded as ADR in Task 10).
func buildInput(table string) *dynamodb.CreateTableInput {
	return &dynamodb.CreateTableInput{
		TableName: aws.String(table),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("Lift"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("Metadata"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("TotalUniqueLiftRiders"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("Lift"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("Metadata"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String(dbpkg.GSIFullName),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("Lift"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("TotalUniqueLiftRiders"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx := context.Background()
	client, err := dbpkg.NewClient(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dynamodb client:", err)
		os.Exit(1)
	}
	_, err = client.CreateTable(ctx, buildInput(cfg.TableName))
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ResourceInUseException" {
			slog.Info("table exists, treating as success", "table", cfg.TableName)
			return
		}
		fmt.Fprintln(os.Stderr, "create table:", err)
		os.Exit(1)
	}
	slog.Info("table created", "table", cfg.TableName)
}
