package handlers_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gin-gonic/gin"

	"gin-ddb/internal/config"
	"gin-ddb/internal/handlers"
	"gin-ddb/internal/repository"
	"gin-ddb/internal/server"
	"gin-ddb/internal/service"
)

// failingAPI fails every DynamoDB call to exercise 503/500 paths.
type failingAPI struct{ err error }

func (f failingAPI) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return nil, f.err
}

func (f failingAPI) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return nil, f.err
}

func (f failingAPI) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return nil, f.err
}

func (f failingAPI) Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	return nil, f.err
}

func (f failingAPI) UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return nil, f.err
}

func (f failingAPI) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return nil, f.err
}

func TestReadyNotReady(t *testing.T) {
	cfg := &config.Config{NodeEnv: "test", RateLimitMax: 100}
	repo := repository.New(failingAPI{err: errors.New("ddb down")}, "SkiLifts")
	e := server.New(cfg, service.New(repo))

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"NOT_READY"`) {
		t.Errorf("body = %s, want NOT_READY envelope", w.Body.String())
	}
}

func directHandlerContext(method, body string, params []gin.Param) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var rdr io.Reader = strings.NewReader(body)
	if body == "" {
		rdr = strings.NewReader("")
	}
	c.Request = httptest.NewRequest(method, "/", rdr)
	c.Params = params
	return c, w
}

func testHandler() *handlers.Handler {
	cfg := &config.Config{TableName: "SkiLifts", Region: "us-east-1"}
	return handlers.New(service.New(repository.NewFakeRepository("SkiLifts")), cfg)
}

func TestBlankLiftGuard(t *testing.T) {
	h := testHandler()
	c, w := directHandlerContext(http.MethodGet, "", []gin.Param{{Key: "lift", Value: "  "}})
	h.ListByLift(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for blank lift", w.Code)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func TestUnreadableBodyIs400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = io.NopCloser(errReader{})
	c.Request = req
	testHandler().CreateStatic(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unreadable body", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid JSON") {
		t.Errorf("body = %s, want invalid JSON detail", w.Body.String())
	}
}
