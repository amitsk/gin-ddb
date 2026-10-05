package service

import (
	"context"
	"testing"

	"gin-ddb/internal/repository"
)

// writeOnlyStore persists creates but always fails reads, simulating
// eventual-consistency lag between PutItem and GetItem.
type writeOnlyStore struct {
	Store
}

func (w writeOnlyStore) Get(context.Context, string, string) (map[string]any, error) {
	return nil, repository.ErrNotFound
}

func TestCreateSurvivesReadLag(t *testing.T) {
	svc := New(writeOnlyStore{Store: repository.NewFakeRepository("SkiLifts")})
	created, serr := svc.CreateStatic(context.Background(), staticRec())
	if serr != nil {
		t.Fatalf("CreateStatic() with lagging reads = %+v, want success", serr)
	}
	if created["Lift"] != "Summit Express" || created["VerticalFeet"] == nil {
		t.Errorf("created = %v, want stored record without re-read", created)
	}
}
