package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKey(t *testing.T) {
	k := Key("Summit Express", "static")
	if k["Lift"] != "Summit Express" || k["Metadata"] != "static" {
		t.Errorf("Key() = %v, want Lift/Metadata pair", k)
	}
}

func TestJSONFieldNames(t *testing.T) {
	raw, err := json.Marshal(DynamicRecord{
		Lift: "A", Metadata: "01/15/24", TotalUniqueLiftRiders: 1,
		SnowCount: 2, LiftStatus: "Open", AvalancheDanger: "Low",
	})
	if err != nil {
		t.Fatalf("Marshal() = %v", err)
	}
	for _, key := range []string{
		`"Lift"`, `"Metadata"`, `"TotalUniqueLiftRiders"`,
		`"SnowCount"`, `"LiftStatus"`, `"AvalancheDanger"`,
	} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("JSON %s missing locked field name %s", raw, key)
		}
	}
}
