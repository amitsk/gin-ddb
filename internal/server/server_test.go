package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gin-ddb/internal/config"
	"gin-ddb/internal/repository"
	"gin-ddb/internal/service"
)

func testConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

func testEngine(cfg *config.Config) *gin.Engine {
	return New(cfg, service.New(repository.NewFakeRepository("SkiLifts")))
}

func TestUnknownRouteIsNotFoundEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	testEngine(testConfig()).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"NOT_FOUND"`) {
		t.Errorf("body = %s, want NOT_FOUND envelope", w.Body.String())
	}
	if w.Header().Get("x-request-id") == "" {
		t.Error("x-request-id missing on error response")
	}
}

func TestRateLimitExceeded(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitMax = 2
	e := testEngine(cfg)
	codes := []int{}
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
		codes = append(codes, w.Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != 429 {
		t.Errorf("statuses = %v, want [200 200 429]", codes)
	}
}

func TestRateLimitDisabledInTestEnv(t *testing.T) {
	cfg := testConfig()
	cfg.NodeEnv = "test"
	cfg.RateLimitMax = 1
	e := testEngine(cfg)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
		if w.Code != 200 {
			t.Fatalf("request %d status = %d, want 200 (limiter off)", i, w.Code)
		}
	}
}

func TestCORSDefaults(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.com")
	testEngine(testConfig()).ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Allow-Credentials = %q, want empty when origin is *", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	testEngine(testConfig()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestGzipLargeResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(GzipMiddleware(1024))
	e.GET("/big", func(c *gin.Context) {
		c.String(http.StatusOK, strings.Repeat("a", 2000))
	})
	e.GET("/small", func(c *gin.Context) {
		c.String(http.StatusOK, "tiny")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	e.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip for 2KB body", w.Header().Get("Content-Encoding"))
	}

	small := httptest.NewRecorder()
	smallReq := httptest.NewRequest(http.MethodGet, "/small", nil)
	smallReq.Header.Set("Accept-Encoding", "gzip")
	e.ServeHTTP(small, smallReq)
	if enc := small.Header().Get("Content-Encoding"); enc == "gzip" {
		t.Errorf("small body got Content-Encoding gzip, want uncompressed")
	}
}
