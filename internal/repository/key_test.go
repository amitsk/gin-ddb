package repository

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestStringKeyPreservesNumericSortKey(t *testing.T) {
	lek := map[string]types.AttributeValue{
		"Lift":                  &types.AttributeValueMemberS{Value: "A"},
		"Metadata":              &types.AttributeValueMemberS{Value: "01/01/24"},
		"TotalUniqueLiftRiders": &types.AttributeValueMemberN{Value: "300"},
	}
	str := StringKey(lek)
	if str["TotalUniqueLiftRiders"] != "300" {
		t.Fatalf("StringKey() = %v, want riders preserved as string", str)
	}
	back := avKey(str)
	n, ok := back["TotalUniqueLiftRiders"].(*types.AttributeValueMemberN)
	if !ok || n.Value != "300" {
		t.Fatalf("avKey() riders = %T %v, want MemberN 300", back["TotalUniqueLiftRiders"], back["TotalUniqueLiftRiders"])
	}
	if s, ok := back["Lift"].(*types.AttributeValueMemberS); !ok || s.Value != "A" {
		t.Errorf("avKey() Lift = %v, want MemberS A", back["Lift"])
	}
}
