package repository

import (
	"context"
	"encoding/json"
	"testing"

	"gin-ddb/internal/models"
)

func toJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func staticRec() models.StaticRecord {
	return models.StaticRecord{
		Lift: "Summit Express", Metadata: "static",
		IsDetachable: false, VerticalFeet: 2500, LiftTime: "8:00",
	}
}

func dynamicRec(lift, meta string, riders, snow int) models.DynamicRecord {
	return models.DynamicRecord{
		Lift: lift, Metadata: meta, TotalUniqueLiftRiders: riders,
		SnowCount: snow, LiftStatus: "Open", AvalancheDanger: "Low",
	}
}

func TestCreateGetRoundTrip(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	ctx := context.Background()
	if err := r.Create(ctx, staticRec()); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	got, err := r.Get(ctx, "Summit Express", "static")
	if err != nil {
		t.Fatalf("Get() = %v, want nil", err)
	}
	if toJSON(t, got) != toJSON(t, map[string]any{
		"Lift": "Summit Express", "Metadata": "static", "IsDetachable": false,
		"VerticalFeet": 2500, "LiftTime": "8:00",
	}) {
		t.Errorf("Get() = %s, want fixture values", toJSON(t, got))
	}
}

func TestDoubleCreateFailsCondition(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	ctx := context.Background()
	if err := r.Create(ctx, staticRec()); err != nil {
		t.Fatalf("first Create() = %v", err)
	}
	err := r.Create(ctx, staticRec())
	if !IsConditionFailed(err) {
		t.Errorf("second Create() = %v, want condition failure", err)
	}
}

func TestGetAbsentIsNotFound(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	if _, err := r.Get(context.Background(), "Ghost", "static"); err != ErrNotFound {
		t.Errorf("Get() absent = %v, want ErrNotFound", err)
	}
}

func TestUpdateRoundTrip(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	ctx := context.Background()
	if err := r.Create(ctx, staticRec()); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	updated, err := r.Update(ctx, "Summit Express", "static",
		map[string]any{"VerticalFeet": 3000})
	if err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}
	if toJSON(t, updated["VerticalFeet"]) != "3000" {
		t.Errorf("VerticalFeet = %s, want 3000", toJSON(t, updated["VerticalFeet"]))
	}
	got, _ := r.Get(ctx, "Summit Express", "static")
	if toJSON(t, got["LiftTime"]) != `"8:00"` {
		t.Errorf("untouched field LiftTime = %s, want preserved", toJSON(t, got["LiftTime"]))
	}
}

func TestUpdateAbsentFailsCondition(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	_, err := r.Update(context.Background(), "Ghost", "static",
		map[string]any{"VerticalFeet": 1})
	if !IsConditionFailed(err) {
		t.Errorf("Update() absent = %v, want condition failure", err)
	}
}

func TestDeleteAbsentFailsCondition(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	if err := r.Delete(context.Background(), "Ghost", "static"); !IsConditionFailed(err) {
		t.Errorf("Delete() absent = %v, want condition failure", err)
	}
}

func TestDeleteRemoves(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	ctx := context.Background()
	if err := r.Create(ctx, staticRec()); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if err := r.Delete(ctx, "Summit Express", "static"); err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	if _, err := r.Get(ctx, "Summit Express", "static"); err != ErrNotFound {
		t.Errorf("Get() after delete = %v, want ErrNotFound", err)
	}
}

func seedRiders(t *testing.T, r *Repository) {
	t.Helper()
	ctx := context.Background()
	for _, d := range []models.DynamicRecord{
		dynamicRec("A", "01/01/24", 100, 10),
		dynamicRec("A", "01/02/24", 300, 10),
		dynamicRec("A", "01/03/24", 200, 10),
	} {
		if err := r.Create(ctx, d); err != nil {
			t.Fatalf("seed Create() = %v", err)
		}
	}
}

func TestQueryByLiftPagination(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	seedRiders(t, r)
	ctx := context.Background()
	page1, lek, err := r.QueryByLift(ctx, "A", 2, nil)
	if err != nil || len(page1) != 2 {
		t.Fatalf("page1 = %d items, %v; want 2, nil", len(page1), err)
	}
	if lek == nil {
		t.Fatal("page1 LastEvaluatedKey = nil, want cursor")
	}
	page2, lek2, err := r.QueryByLift(ctx, "A", 2, lek)
	if err != nil || len(page2) != 1 || lek2 != nil {
		t.Fatalf("page2 = %d items, lek=%v, %v; want 1, nil, nil", len(page2), lek2, err)
	}
}

func TestScanPagination(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	seedRiders(t, r)
	ctx := context.Background()
	if err := r.Create(ctx, staticRec()); err != nil {
		t.Fatalf("Create() = %v", err)
	}
	page1, lek, err := r.Scan(ctx, 3, nil)
	if err != nil || len(page1) != 3 || lek == nil {
		t.Fatalf("page1 = %d items, lek=%v, %v; want 3, cursor, nil", len(page1), lek, err)
	}
	page2, _, err := r.Scan(ctx, 3, lek)
	if err != nil || len(page2) != 1 {
		t.Fatalf("page2 = %d items, %v; want 1, nil", len(page2), err)
	}
}

func TestQueryByRidersBranches(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	seedRiders(t, r)
	ctx := context.Background()

	all, _, err := r.QueryByRiders(ctx, "A", nil, nil, 10, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("unbounded = %d items, %v; want 3", len(all), err)
	}
	// Descending by riders: 300, 200, 100.
	if toJSON(t, all[0]["Metadata"]) != `"01/02/24"` ||
		toJSON(t, all[1]["Metadata"]) != `"01/03/24"` ||
		toJSON(t, all[2]["Metadata"]) != `"01/01/24"` {
		t.Errorf("order wrong: %s %s %s", toJSON(t, all[0]["Metadata"]), toJSON(t, all[1]["Metadata"]), toJSON(t, all[2]["Metadata"]))
	}

	bounded, _, _ := r.QueryByRiders(ctx, "A", intPtr(150), intPtr(250), 10, nil)
	if len(bounded) != 1 || toJSON(t, bounded[0]["Metadata"]) != `"01/03/24"` {
		t.Errorf("BETWEEN 150..250 = %v, want [01/03/24]", bounded)
	}
	minOnly, _, _ := r.QueryByRiders(ctx, "A", intPtr(200), nil, 10, nil)
	if len(minOnly) != 2 {
		t.Errorf(">= 200 = %d items, want 2", len(minOnly))
	}
	maxOnly, _, _ := r.QueryByRiders(ctx, "A", nil, intPtr(200), 10, nil)
	if len(maxOnly) != 2 {
		t.Errorf("<= 200 = %d items, want 2", len(maxOnly))
	}
}

func TestProbe(t *testing.T) {
	r := NewFakeRepository("SkiLifts")
	if err := r.Probe(context.Background()); err != nil {
		t.Errorf("Probe() = %v, want nil even on empty table", err)
	}
}

func intPtr(n int) *int { return &n }
