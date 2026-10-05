// Package repository owns DynamoDB table I/O only (§7). Domain rules
// live in the service; the client is a narrow interface so tests inject
// fakes (NewFakeRepository). Condition failures pass through raw for
// apperrors.MapDynamoDB; absent reads surface ErrNotFound.
package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"

	dbpkg "gin-ddb/internal/db"
)

// ErrNotFound signals an absent item on read paths.
var ErrNotFound = errors.New("repository: item not found")

const (
	existsCondition   = "attribute_exists(Lift) AND attribute_exists(Metadata)"
	notExistsCondition = "attribute_not_exists(Lift) AND attribute_not_exists(Metadata)"
)

// DynamoDBAPI is the narrow client surface this package needs.
type DynamoDBAPI interface {
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

// Repository wraps a table name and client.
type Repository struct {
	client DynamoDBAPI
	table  string
}

// New returns a Repository over a real client.
func New(client DynamoDBAPI, table string) *Repository {
	return &Repository{client: client, table: table}
}

// IsConditionFailed reports a failed exists-condition from any backend.
func IsConditionFailed(err error) bool {
	var apiErr smithy.APIError
	return err != nil && errors.As(err, &apiErr) &&
		apiErr.ErrorCode() == "ConditionalCheckFailedException"
}

func keyAV(lift, meta string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"Lift":     &types.AttributeValueMemberS{Value: lift},
		"Metadata": &types.AttributeValueMemberS{Value: meta},
	}
}

// StringKey converts SDK key attributes to raw values for pagination.
func StringKey(lek map[string]types.AttributeValue) map[string]string {
	if len(lek) == 0 {
		return nil
	}
	out := make(map[string]string, len(lek))
	for k, v := range lek {
		if s, ok := v.(*types.AttributeValueMemberS); ok {
			out[k] = s.Value
		}
	}
	return out
}

// avKey converts pagination values back to SDK key attributes.
func avKey(key map[string]string) map[string]types.AttributeValue {
	if len(key) == 0 {
		return nil
	}
	out := make(map[string]types.AttributeValue, len(key))
	for k, v := range key {
		out[k] = &types.AttributeValueMemberS{Value: v}
	}
	return out
}

func unmarshalItem(item map[string]types.AttributeValue) (map[string]any, error) {
	var out map[string]any
	if err := attributevalue.UnmarshalMap(item, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Create puts a record only when the key is absent (→409 via mapper).
func (r *Repository) Create(ctx context.Context, record any) error {
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return err
	}
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                item,
		ConditionExpression: aws.String(notExistsCondition),
	})
	return err
}

// Get fetches one item; absent → ErrNotFound.
func (r *Repository) Get(ctx context.Context, lift, meta string) (map[string]any, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key:       keyAV(lift, meta),
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, ErrNotFound
	}
	return unmarshalItem(out.Item)
}

// Update applies a partial SET and returns ALL_NEW; absent → condition failure.
func (r *Repository) Update(ctx context.Context, lift, meta string, updates map[string]any) (map[string]any, error) {
	if len(updates) == 0 {
		return nil, fmt.Errorf("repository: empty update")
	}
	names := make([]string, 0, len(updates))
	for name := range updates {
		names = append(names, name)
	}
	sort.Strings(names)
	set := make([]string, 0, len(names))
	attrNames := make(map[string]string, len(names))
	attrValues := make(map[string]types.AttributeValue, len(names))
	for i, name := range names {
		ph, pv := fmt.Sprintf("#f%d", i), fmt.Sprintf(":v%d", i)
		set = append(set, ph+"="+pv)
		attrNames[ph] = name
		av, err := attributevalue.Marshal(updates[name])
		if err != nil {
			return nil, err
		}
		attrValues[pv] = av
	}
	out, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(r.table),
		Key:                       keyAV(lift, meta),
		UpdateExpression:          aws.String("SET " + joinComma(set)),
		ExpressionAttributeNames:  attrNames,
		ExpressionAttributeValues: attrValues,
		ConditionExpression:       aws.String(existsCondition),
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		return nil, err
	}
	return unmarshalItem(out.Attributes)
}

// Delete removes one item; absent → condition failure (404 via mapper).
func (r *Repository) Delete(ctx context.Context, lift, meta string) error {
	_, err := r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(r.table),
		Key:                 keyAV(lift, meta),
		ConditionExpression: aws.String(existsCondition),
	})
	return err
}

// pageQuery runs a Query/Scan page and converts the cursor.
func pageQuery(
	items []map[string]types.AttributeValue,
	lek map[string]types.AttributeValue,
) ([]map[string]any, map[string]string, error) {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, err := unmarshalItem(item)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, m)
	}
	return out, StringKey(lek), nil
}

// QueryByLift lists one lift's collection (PK query, SK ascending).
func (r *Repository) QueryByLift(ctx context.Context, lift string, limit int, start map[string]string) ([]map[string]any, map[string]string, error) {
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("Lift = :lift"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":lift": &types.AttributeValueMemberS{Value: lift},
		},
		Limit:             aws.Int32(int32(limit)),
		ExclusiveStartKey: avKey(start),
	})
	if err != nil {
		return nil, nil, err
	}
	return pageQuery(out.Items, out.LastEvaluatedKey)
}

// Scan lists the whole table.
func (r *Repository) Scan(ctx context.Context, limit int, start map[string]string) ([]map[string]any, map[string]string, error) {
	out, err := r.client.Scan(ctx, &dynamodb.ScanInput{
		TableName:         aws.String(r.table),
		Limit:             aws.Int32(int32(limit)),
		ExclusiveStartKey: avKey(start),
	})
	if err != nil {
		return nil, nil, err
	}
	return pageQuery(out.Items, out.LastEvaluatedKey)
}

// QueryByRiders runs the GSI query, descending, with min/max branches.
func (r *Repository) QueryByRiders(ctx context.Context, lift string, min, max *int, limit int, start map[string]string) ([]map[string]any, map[string]string, error) {
	vals := map[string]types.AttributeValue{
		":lift": &types.AttributeValueMemberS{Value: lift},
	}
	keyCond := "Lift = :lift"
	switch {
	case min != nil && max != nil:
		keyCond += " AND TotalUniqueLiftRiders BETWEEN :min AND :max"
		vals[":min"] = &types.AttributeValueMemberN{Value: strconv.Itoa(*min)}
		vals[":max"] = &types.AttributeValueMemberN{Value: strconv.Itoa(*max)}
	case min != nil:
		keyCond += " AND TotalUniqueLiftRiders >= :min"
		vals[":min"] = &types.AttributeValueMemberN{Value: strconv.Itoa(*min)}
	case max != nil:
		keyCond += " AND TotalUniqueLiftRiders <= :max"
		vals[":max"] = &types.AttributeValueMemberN{Value: strconv.Itoa(*max)}
	}
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		IndexName:              aws.String(dbpkg.GSIFullName),
		KeyConditionExpression: aws.String(keyCond),
		ExpressionAttributeValues: vals,
		ScanIndexForward:       aws.Bool(false),
		Limit:                  aws.Int32(int32(limit)),
		ExclusiveStartKey:      avKey(start),
	})
	if err != nil {
		return nil, nil, err
	}
	return pageQuery(out.Items, out.LastEvaluatedKey)
}

// Probe checks table reachability for /ready.
func (r *Repository) Probe(ctx context.Context) error {
	_, err := r.client.Scan(ctx, &dynamodb.ScanInput{
		TableName:            aws.String(r.table),
		Limit:                aws.Int32(1),
		ProjectionExpression: aws.String("Lift"),
	})
	return err
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
