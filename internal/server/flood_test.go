package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiterSurvivesIPFlood(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitMax = 2
	cfg.RateLimitWindow = time.Hour
	e := testEngine(cfg)

	// One request per unique IP past the bucket cap: all fresh, all pass.
	for i := 0; i < maxBuckets+5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.9.%d.%d", i/256, i%256))
		e.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("flood ip %d status = %d, want 200", i, w.Code)
		}
	}
	// Normal limiting still trips for a repeat offender afterwards
	// (fresh IP unseen in the flood).
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Forwarded-For", "10.10.10.10")
		e.ServeHTTP(w, req)
		if i < 2 && w.Code != http.StatusOK {
			t.Fatalf("repeat %d status = %d, want 200", i, w.Code)
		}
		if i == 2 && w.Code != http.StatusTooManyRequests {
			t.Fatalf("repeat %d status = %d, want 429", i, w.Code)
		}
	}
}
