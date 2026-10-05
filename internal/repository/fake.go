package repository

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
)

// fakeDB is an in-memory DynamoDBAPI honoring exactly the expressions
// this Repository generates (conditions, key conditions, SET updates).
type fakeDB struct {
	items map[string]map[string]types.AttributeValue
}

func condFailed() error {
	return &smithy.GenericAPIError{
		Code: "ConditionalCheckFailedException", Message: "condition failed",
	}
}

func strOf(av types.AttributeValue) (string, bool) {
	s, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return "", false
	}
	return s.Value, true
}

func numOf(av types.AttributeValue) (int, bool) {
	n, ok := av.(*types.AttributeValueMemberN)
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(n.Value)
	return v, err == nil
}

func itemKey(item map[string]types.AttributeValue) (lift, meta string, ok bool) {
	lift, ok1 := strOf(item["Lift"])
	meta, ok2 := strOf(item["Metadata"])
	return lift, meta, ok1 && ok2
}

func storeKey(lift, meta string) string { return lift + "\x00" + meta }

func copyItem(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	out := make(map[string]types.AttributeValue, len(item))
	for k, v := range item {
		out[k] = v
	}
	return out
}

func (f *fakeDB) PutItem(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	lift, meta, ok := itemKey(in.Item)
	if !ok {
		return nil, &smithy.GenericAPIError{Code: "ValidationException", Message: "missing key"}
	}
	if in.ConditionExpression != nil && *in.ConditionExpression != "" {
		if _, exists := f.items[storeKey(lift, meta)]; exists {
			return nil, condFailed()
		}
	}
	f.items[storeKey(lift, meta)] = copyItem(in.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func (f *fakeDB) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	lift, _ := strOf(in.Key["Lift"])
	meta, _ := strOf(in.Key["Metadata"])
	item, ok := f.items[storeKey(lift, meta)]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copyItem(item)}, nil
}

func keyMatches(key map[string]types.AttributeValue, lift, meta string) bool {
	l, _ := strOf(key["Lift"])
	m, _ := strOf(key["Metadata"])
	return l == lift && m == meta
}

// paginate slices sorted items at the start cursor with limit, returning
// the page and the SDK LastEvaluatedKey when more rows remain.
func paginate(sorted []map[string]types.AttributeValue, limit int32, start map[string]types.AttributeValue) ([]map[string]types.AttributeValue, map[string]types.AttributeValue) {
	from := 0
	if len(start) > 0 {
		lift, _ := strOf(start["Lift"])
		meta, _ := strOf(start["Metadata"])
		for i, item := range sorted {
			if keyMatches(item, lift, meta) {
				from = i + 1
				break
			}
		}
	}
	rest := sorted[from:]
	var lek map[string]types.AttributeValue
	if limit > 0 && int(limit) < len(rest) {
		rest = rest[:limit]
		last := rest[len(rest)-1]
		lek = map[string]types.AttributeValue{"Lift": last["Lift"], "Metadata": last["Metadata"]}
	}
	return rest, lek
}

func sortBySK(items []map[string]types.AttributeValue) {
	sort.Slice(items, func(i, j int) bool {
		li, _ := strOf(items[i]["Lift"])
		lj, _ := strOf(items[j]["Lift"])
		if li != lj {
			return li < lj
		}
		mi, _ := strOf(items[i]["Metadata"])
		mj, _ := strOf(items[j]["Metadata"])
		return mi < mj
	})
}

func (f *fakeDB) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	lift, _ := strOf(in.ExpressionAttributeValues[":lift"])
	var rows []map[string]types.AttributeValue
	if in.IndexName != nil && *in.IndexName != "" {
		// GSI: only items carrying the rider sort key.
		for _, item := range f.items {
			l, _ := strOf(item["Lift"])
			if l != lift {
				continue
			}
			if _, ok := numOf(item["TotalUniqueLiftRiders"]); !ok {
				continue
			}
			rows = append(rows, copyItem(item))
		}
		rows = filterRiders(rows, in)
		sort.Slice(rows, func(i, j int) bool {
			ri, _ := numOf(rows[i]["TotalUniqueLiftRiders"])
			rj, _ := numOf(rows[j]["TotalUniqueLiftRiders"])
			if ri != rj {
				return ri > rj
			}
			mi, _ := strOf(rows[i]["Metadata"])
			mj, _ := strOf(rows[j]["Metadata"])
			return mi < mj
		})
	} else {
		for _, item := range f.items {
			if l, _ := strOf(item["Lift"]); l == lift {
				rows = append(rows, copyItem(item))
			}
		}
		sortBySK(rows)
	}
	var limit int32
	if in.Limit != nil {
		limit = *in.Limit
	}
	page, lek := paginate(rows, limit, in.ExclusiveStartKey)
	return &dynamodb.QueryOutput{Items: page, LastEvaluatedKey: lek}, nil
}

func filterRiders(rows []map[string]types.AttributeValue, in *dynamodb.QueryInput) []map[string]types.AttributeValue {
	cond := ""
	if in.KeyConditionExpression != nil {
		cond = *in.KeyConditionExpression
	}
	bound := func(name string) (int, bool) {
		av, ok := in.ExpressionAttributeValues[name]
		if !ok {
			return 0, false
		}
		return numOf(av)
	}
	out := rows[:0]
	for _, item := range rows {
		riders, _ := numOf(item["TotalUniqueLiftRiders"])
		keep := true
		if strings.Contains(cond, "BETWEEN") {
			lo, _ := bound(":min")
			hi, _ := bound(":max")
			keep = riders >= lo && riders <= hi
		} else if strings.Contains(cond, ">=") {
			lo, _ := bound(":min")
			keep = riders >= lo
		} else if strings.Contains(cond, "<=") {
			hi, _ := bound(":max")
			keep = riders <= hi
		}
		if keep {
			out = append(out, item)
		}
	}
	return out
}

func (f *fakeDB) Scan(_ context.Context, in *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	var rows []map[string]types.AttributeValue
	for _, item := range f.items {
		c := copyItem(item)
		if in.ProjectionExpression != nil && *in.ProjectionExpression == "Lift" {
			c = map[string]types.AttributeValue{"Lift": item["Lift"]}
		}
		rows = append(rows, c)
	}
	sortBySK(rows)
	var limit int32
	if in.Limit != nil {
		limit = *in.Limit
	}
	page, lek := paginate(rows, limit, in.ExclusiveStartKey)
	return &dynamodb.ScanOutput{Items: page, LastEvaluatedKey: lek}, nil
}

func (f *fakeDB) UpdateItem(_ context.Context, in *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	lift, _ := strOf(in.Key["Lift"])
	meta, _ := strOf(in.Key["Metadata"])
	item, ok := f.items[storeKey(lift, meta)]
	if in.ConditionExpression != nil && *in.ConditionExpression != "" && !ok {
		return nil, condFailed()
	}
	if !ok {
		item = keyAV(lift, meta)
	}
	updated := copyItem(item)
	expr := ""
	if in.UpdateExpression != nil {
		expr = strings.TrimPrefix(*in.UpdateExpression, "SET ")
	}
	for _, assign := range strings.Split(expr, ",") {
		kv := strings.SplitN(strings.TrimSpace(assign), "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := in.ExpressionAttributeNames[strings.TrimSpace(kv[0])]
		val := in.ExpressionAttributeValues[strings.TrimSpace(kv[1])]
		if name == "" || val == nil {
			continue
		}
		updated[name] = val
	}
	f.items[storeKey(lift, meta)] = updated
	return &dynamodb.UpdateItemOutput{Attributes: copyItem(updated)}, nil
}

func (f *fakeDB) DeleteItem(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	lift, _ := strOf(in.Key["Lift"])
	meta, _ := strOf(in.Key["Metadata"])
	if in.ConditionExpression != nil && *in.ConditionExpression != "" {
		if _, ok := f.items[storeKey(lift, meta)]; !ok {
			return nil, condFailed()
		}
	}
	delete(f.items, storeKey(lift, meta))
	return &dynamodb.DeleteItemOutput{}, nil
}

// NewFakeRepository returns a Repository backed by in-memory storage.
func NewFakeRepository(table string) *Repository {
	return New(&fakeDB{items: map[string]map[string]types.AttributeValue{}}, table)
}
