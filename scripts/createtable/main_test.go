package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func strVal(in *string) string {
	if in == nil {
		return ""
	}
	return *in
}

func TestBuildInputKeysAndGSI(t *testing.T) {
	in := buildInput("SkiLifts")
	if strVal(in.TableName) != "SkiLifts" {
		t.Errorf("TableName = %q, want SkiLifts", strVal(in.TableName))
	}
	keyNames := map[string]types.KeyType{}
	for _, k := range in.KeySchema {
		keyNames[strVal(k.AttributeName)] = k.KeyType
	}
	if keyNames["Lift"] != types.KeyTypeHash || keyNames["Metadata"] != types.KeyTypeRange {
		t.Errorf("KeySchema = %v, want Lift=HASH Metadata=RANGE", in.KeySchema)
	}
	for _, name := range []string{"Lift", "Metadata", "TotalUniqueLiftRiders"} {
		found := false
		for _, def := range in.AttributeDefinitions {
			if strVal(def.AttributeName) == name {
				found = true
				if name == "TotalUniqueLiftRiders" && def.AttributeType != types.ScalarAttributeTypeN {
					t.Errorf("TotalUniqueLiftRiders type = %v, want N", def.AttributeType)
				}
				if name != "TotalUniqueLiftRiders" && def.AttributeType != types.ScalarAttributeTypeS {
					t.Errorf("%s type = %v, want S", name, def.AttributeType)
				}
			}
		}
		if !found {
			t.Errorf("AttributeDefinitions missing %q", name)
		}
	}
	if len(in.GlobalSecondaryIndexes) != 1 {
		t.Fatalf("GSI count = %d, want 1", len(in.GlobalSecondaryIndexes))
	}
	gsi := in.GlobalSecondaryIndexes[0]
	if strVal(gsi.IndexName) != "SkiLiftsByRiders" {
		t.Errorf("IndexName = %q, want SkiLiftsByRiders", strVal(gsi.IndexName))
	}
	if gsi.Projection == nil || gsi.Projection.ProjectionType != types.ProjectionTypeAll {
		t.Errorf("GSI projection = %+v, want ALL", gsi.Projection)
	}
	gsiKeys := map[string]types.KeyType{}
	for _, k := range gsi.KeySchema {
		gsiKeys[strVal(k.AttributeName)] = k.KeyType
	}
	if gsiKeys["Lift"] != types.KeyTypeHash || gsiKeys["TotalUniqueLiftRiders"] != types.KeyTypeRange {
		t.Errorf("GSI keys = %v, want Lift=HASH TotalUniqueLiftRiders=RANGE", gsi.KeySchema)
	}
}

func TestBuildInputOnDemandBilling(t *testing.T) {
	in := buildInput("SkiLifts")
	if in.BillingMode != types.BillingModePayPerRequest {
		t.Errorf("BillingMode = %v, want PAY_PER_REQUEST", in.BillingMode)
	}
}
