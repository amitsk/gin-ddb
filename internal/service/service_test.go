package service

import (
	"context"
	"net/http"
	"testing"

	"gin-ddb/internal/apperrors"
	"gin-ddb/internal/models"
	"gin-ddb/internal/repository"
)

func testService() *Service {
	return New(repository.NewFakeRepository("SkiLifts"))
}

func staticRec() models.StaticRecord {
	return models.StaticRecord{
		Lift: "Summit Express", Metadata: "static",
		IsDetachable: false, VerticalFeet: 2500, LiftTime: "8:00",
	}
}

func dynamicRec() models.DynamicRecord {
	return models.DynamicRecord{
		Lift: "Summit Express", Metadata: "01/15/24",
		TotalUniqueLiftRiders: 1250, SnowCount: 48,
		LiftStatus: "Open", AvalancheDanger: "Low",
	}
}

func TestCreateGet(t *testing.T) {
	s := testService()
	ctx := context.Background()
	created, serr := s.CreateStatic(ctx, staticRec())
	if serr != nil {
		t.Fatalf("CreateStatic() = %+v, want nil", serr)
	}
	if created["Lift"] != "Summit Express" {
		t.Errorf("created = %v, want fixture", created)
	}
	got, serr := s.Get(ctx, "Summit Express", "static")
	if serr != nil || got["VerticalFeet"] == nil {
		t.Errorf("Get() = %v, %+v; want item", got, serr)
	}
}

func TestDoubleCreateConflicts(t *testing.T) {
	s := testService()
	ctx := context.Background()
	if _, serr := s.CreateStatic(ctx, staticRec()); serr != nil {
		t.Fatalf("first CreateStatic() = %+v", serr)
	}
	_, serr := s.CreateStatic(ctx, staticRec())
	if serr == nil || serr.Code != apperrors.CodeConflict || serr.Status != http.StatusConflict {
		t.Errorf("second CreateStatic() = %+v, want (CONFLICT,409)", serr)
	}
}

func TestGetAbsentNotFound(t *testing.T) {
	_, serr := testService().Get(context.Background(), "Ghost", "static")
	if serr == nil || serr.Code != apperrors.CodeNotFound || serr.Status != http.StatusNotFound {
		t.Errorf("Get() absent = %+v, want (NOT_FOUND,404)", serr)
	}
}

func TestUpdateMerges(t *testing.T) {
	s := testService()
	ctx := context.Background()
	if _, serr := s.CreateDynamic(ctx, dynamicRec()); serr != nil {
		t.Fatalf("CreateDynamic() = %+v", serr)
	}
	updated, serr := s.Update(ctx, "Summit Express", "01/15/24",
		map[string]any{"SnowCount": 60})
	if serr != nil {
		t.Fatalf("Update() = %+v, want nil", serr)
	}
	if updated["SnowCount"] == nil || updated["LiftStatus"] == nil {
		t.Errorf("Update() = %v, want merged ALL_NEW", updated)
	}
}

func TestUpdateDeleteAbsentNotFound(t *testing.T) {
	s := testService()
	ctx := context.Background()
	if _, serr := s.Update(ctx, "Ghost", "static", map[string]any{"VerticalFeet": 1}); serr == nil ||
		serr.Code != apperrors.CodeNotFound {
		t.Errorf("Update() absent = %+v, want NOT_FOUND", serr)
	}
	if serr := s.Delete(ctx, "Ghost", "static"); serr == nil ||
		serr.Code != apperrors.CodeNotFound {
		t.Errorf("Delete() absent = %+v, want NOT_FOUND", serr)
	}
}

func TestDeleteRemoves(t *testing.T) {
	s := testService()
	ctx := context.Background()
	if _, serr := s.CreateStatic(ctx, staticRec()); serr != nil {
		t.Fatalf("CreateStatic() = %+v", serr)
	}
	if serr := s.Delete(ctx, "Summit Express", "static"); serr != nil {
		t.Fatalf("Delete() = %+v, want nil", serr)
	}
	if _, serr := s.Get(ctx, "Summit Express", "static"); serr == nil {
		t.Error("Get() after delete = nil, want NOT_FOUND")
	}
}

func TestListPagination(t *testing.T) {
	s := testService()
	ctx := context.Background()
	if _, serr := s.CreateStatic(ctx, staticRec()); serr != nil {
		t.Fatalf("CreateStatic() = %+v", serr)
	}
	if _, serr := s.CreateDynamic(ctx, dynamicRec()); serr != nil {
		t.Fatalf("CreateDynamic() = %+v", serr)
	}
	page, serr := s.ListLift(ctx, "Summit Express", 1, nil)
	if serr != nil || len(page.Items) != 1 || !page.HasNext || page.Count != 1 {
		t.Fatalf("page1 = %+v, %+v; want 1 item with next", page, serr)
	}
	page2, serr := s.ListLift(ctx, "Summit Express", 1, page.LastEvaluatedKey)
	if serr != nil || len(page2.Items) != 1 || page2.HasNext {
		t.Fatalf("page2 = %+v, %+v; want 1 item, no next", page2, serr)
	}
}

func TestListByRiders(t *testing.T) {
	s := testService()
	ctx := context.Background()
	mk := func(meta string, riders int) models.DynamicRecord {
		d := dynamicRec()
		d.Metadata, d.TotalUniqueLiftRiders = meta, riders
		return d
	}
	for _, d := range []models.DynamicRecord{mk("01/01/24", 100), mk("01/02/24", 300)} {
		if _, serr := s.CreateDynamic(ctx, d); serr != nil {
			t.Fatalf("CreateDynamic() = %+v", serr)
		}
	}
	lo, hi := 200, 400
	page, serr := s.ListByRiders(ctx, "Summit Express", &lo, &hi, 10, nil)
	if serr != nil || len(page.Items) != 1 {
		t.Fatalf("bounded = %+v, %+v; want 1 item", page, serr)
	}
}

func resortRec() models.ResortRecord {
	return models.ResortRecord{
		Lift: "Summit Resort", Metadata: "resort",
		TotalRiders: 8500, TotalSnow: 42,
		AvalancheDanger: "Moderate",
		OpenLifts:       []int{1, 2, 3},
	}
}

func TestCreateResortRoundTripAndConflict(t *testing.T) {
	s := testService()
	ctx := context.Background()
	created, serr := s.CreateResort(ctx, resortRec())
	if serr != nil {
		t.Fatalf("CreateResort() = %+v, want nil", serr)
	}
	if created["TotalRiders"] == nil || created["OpenLifts"] == nil {
		t.Errorf("created = %v, want resort fields", created)
	}
	if _, serr := s.CreateResort(ctx, resortRec()); serr == nil ||
		serr.Code != apperrors.CodeConflict || serr.Status != http.StatusConflict {
		t.Errorf("second CreateResort() = %+v, want (CONFLICT,409)", serr)
	}
}

func TestServiceErrorString(t *testing.T) {
	serr := &Error{Code: apperrors.CodeNotFound, Status: 404, Message: "item not found"}
	if got := serr.Error(); got != "NOT_FOUND: item not found" {
		t.Errorf("Error() = %q, want code-prefixed message", got)
	}
}
