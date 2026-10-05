// Package service holds domain rules only (§7): no HTTP, no DynamoDB
// SDK types. It translates repository outcomes into service errors that
// carry the §9 code and HTTP status for the handlers to render.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"gin-ddb/internal/apperrors"
	"gin-ddb/internal/models"
	"gin-ddb/internal/repository"
)

// Error is a domain failure with its §9 code, HTTP status, and message.
type Error struct {
	Code    string
	Status  int
	Message string
	Details []apperrors.Detail
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Page is one collection slice with its follow-up cursor.
type Page struct {
	Items            []map[string]any
	HasNext          bool
	LastEvaluatedKey map[string]string
	Count            int
}

// Service wires the repository to domain rules.
type Service struct {
	repo Store
}

// Store is the narrow table-I/O surface the service needs. The real
// *repository.Repository satisfies it; tests may substitute fakes or
// stubs without touching DynamoDB mapping code.
type Store interface {
	Create(ctx context.Context, record any) error
	Get(ctx context.Context, lift, meta string) (map[string]any, error)
	Update(ctx context.Context, lift, meta string, updates map[string]any) (map[string]any, error)
	Delete(ctx context.Context, lift, meta string) error
	Scan(ctx context.Context, limit int, start map[string]string) ([]map[string]any, map[string]string, error)
	QueryByLift(ctx context.Context, lift string, limit int, start map[string]string) ([]map[string]any, map[string]string, error)
	QueryByRiders(ctx context.Context, lift string, min, max *int, limit int, start map[string]string) ([]map[string]any, map[string]string, error)
	Probe(ctx context.Context) error
}

var _ Store = (*repository.Repository)(nil)

// New returns a Service over repo.
func New(repo Store) *Service { return &Service{repo: repo} }

func fromRepo(err error, isCreate bool) *Error {
	if errors.Is(err, repository.ErrNotFound) {
		return &Error{Code: apperrors.CodeNotFound, Status: http.StatusNotFound, Message: "item not found"}
	}
	code, status := apperrors.MapDynamoDB(err, isCreate)
	msg := apperrors.InternalErrorMessage
	switch code {
	case apperrors.CodeConflict:
		msg = "item already exists"
	case apperrors.CodeNotFound:
		msg = "item not found"
	case apperrors.CodeThrottled:
		msg = "service throttled, retry later"
	}
	return &Error{Code: code, Status: status, Message: msg}
}

// storedForm renders the validated record as stored. Creates never
// re-read: DynamoDB default reads are eventually consistent, so a Get
// after Put can 404 a record that was just created.
func storedForm(record any) (map[string]any, *Error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, &Error{Code: apperrors.CodeInternalError, Status: http.StatusInternalServerError, Message: apperrors.InternalErrorMessage}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &Error{Code: apperrors.CodeInternalError, Status: http.StatusInternalServerError, Message: apperrors.InternalErrorMessage}
	}
	return out, nil
}

// CreateStatic creates a static record; existing → 409.
func (s *Service) CreateStatic(ctx context.Context, rec models.StaticRecord) (map[string]any, *Error) {
	if err := s.repo.Create(ctx, rec); err != nil {
		return nil, fromRepo(err, true)
	}
	return storedForm(rec)
}

// CreateDynamic creates a dynamic record; existing → 409.
func (s *Service) CreateDynamic(ctx context.Context, rec models.DynamicRecord) (map[string]any, *Error) {
	if err := s.repo.Create(ctx, rec); err != nil {
		return nil, fromRepo(err, true)
	}
	return storedForm(rec)
}

// CreateResort creates a resort record; existing → 409.
func (s *Service) CreateResort(ctx context.Context, rec models.ResortRecord) (map[string]any, *Error) {
	if err := s.repo.Create(ctx, rec); err != nil {
		return nil, fromRepo(err, true)
	}
	return storedForm(rec)
}

// Get fetches one item; absent → 404.
func (s *Service) Get(ctx context.Context, lift, meta string) (map[string]any, *Error) {
	item, err := s.repo.Get(ctx, lift, meta)
	if err != nil {
		return nil, fromRepo(err, false)
	}
	return item, nil
}

// Update applies a partial update; absent → 404.
func (s *Service) Update(ctx context.Context, lift, meta string, updates map[string]any) (map[string]any, *Error) {
	updated, err := s.repo.Update(ctx, lift, meta, updates)
	if err != nil {
		return nil, fromRepo(err, false)
	}
	return updated, nil
}

// Delete removes one item; absent → 404.
func (s *Service) Delete(ctx context.Context, lift, meta string) *Error {
	if err := s.repo.Delete(ctx, lift, meta); err != nil {
		return fromRepo(err, false)
	}
	return nil
}

func toPage(items []map[string]any, lek map[string]string) Page {
	return Page{Items: items, LastEvaluatedKey: lek, HasNext: lek != nil, Count: len(items)}
}

// ListAll scans one page of the table.
func (s *Service) ListAll(ctx context.Context, limit int, start map[string]string) (Page, *Error) {
	items, lek, err := s.repo.Scan(ctx, limit, start)
	if err != nil {
		return Page{}, fromRepo(err, false)
	}
	return toPage(items, lek), nil
}

// ListLift queries one lift's collection.
func (s *Service) ListLift(ctx context.Context, lift string, limit int, start map[string]string) (Page, *Error) {
	items, lek, err := s.repo.QueryByLift(ctx, lift, limit, start)
	if err != nil {
		return Page{}, fromRepo(err, false)
	}
	return toPage(items, lek), nil
}

// ListByRiders queries the GSI rider-ordered page for one lift.
func (s *Service) ListByRiders(ctx context.Context, lift string, min, max *int, limit int, start map[string]string) (Page, *Error) {
	items, lek, err := s.repo.QueryByRiders(ctx, lift, min, max, limit, start)
	if err != nil {
		return Page{}, fromRepo(err, false)
	}
	return toPage(items, lek), nil
}

// Probe checks table reachability for /ready.
func (s *Service) Probe(ctx context.Context) error { return s.repo.Probe(ctx) }
