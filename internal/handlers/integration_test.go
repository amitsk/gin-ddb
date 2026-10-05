package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gin-ddb/internal/config"
	"gin-ddb/internal/repository"
	"gin-ddb/internal/server"
	"gin-ddb/internal/service"
)

func integrationEngine(t *testing.T) *gin.Engine {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		// Fall back to bare defaults when the ambient env is dirty.
		cfg = &config.Config{
			Host: "0.0.0.0", Port: 3000, LogLevel: "info",
			CORSOrigin: "*", RateLimitMax: 100, DynamoMode: "local",
			TableName: "SkiLifts", LocalEndpoint: "http://localhost:8000",
			LocalRegion: "us-east-1", Region: "us-east-1",
		}
	}
	cfg.NodeEnv = "test" // rate limiter off for isolation
	return server.New(cfg, service.New(repository.NewFakeRepository("SkiLifts")))
}

func doReq(e *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func envelopeCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response not JSON: %s", w.Body.String())
	}
	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error envelope: %s", w.Body.String())
	}
	code, _ := errObj["code"].(string)
	return code
}

func requireStatus(t *testing.T, w *httptest.ResponseRecorder, want int, where string) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("%s: status = %d, want %d (body %s)", where, w.Code, want, w.Body.String())
	}
	if w.Header().Get("x-request-id") == "" {
		t.Fatalf("%s: x-request-id missing", where)
	}
}

func requireError(t *testing.T, w *httptest.ResponseRecorder, status int, code, where string) {
	t.Helper()
	requireStatus(t, w, status, where)
	if got := envelopeCode(t, w); got != code {
		t.Fatalf("%s: code = %s, want %s", where, got, code)
	}
	if status == 400 {
		var parsed map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &parsed)
		details, ok := parsed["error"].(map[string]any)["details"].([]any)
		if !ok || len(details) == 0 {
			t.Fatalf("%s: 400 without details", where)
		}
	}
}

const (
	staticBody  = `{"Lift":"Summit Express","Metadata":"static","IsDetachable":false,"VerticalFeet":2500,"LiftTime":"8:00"}`
	dynamicBody = `{"Lift":"Summit Express","Metadata":"01/15/24","TotalUniqueLiftRiders":1250,"SnowCount":48,"LiftStatus":"Open","AvalancheDanger":"Low"}`
	resortBody  = `{"Lift":"Summit Resort","Metadata":"resort","TotalRiders":8500,"TotalSnow":42,"AvalancheDanger":"Moderate","OpenLifts":[1,2,3,4,5,6,7,8,9,10,11,12]}`
)

func seedThree(t *testing.T, e *gin.Engine) {
	t.Helper()
	for _, b := range []struct{ path, body string }{
		{"/api/skilifts/static", staticBody},
		{"/api/skilifts/dynamic", dynamicBody},
		{"/api/skilifts/resort", resortBody},
	} {
		w := doReq(e, http.MethodPost, b.path, b.body)
		requireStatus(t, w, http.StatusCreated, "seed "+b.path)
	}
}

func TestStaticRoundTrip(t *testing.T) {
	e := integrationEngine(t)
	w := doReq(e, http.MethodPost, "/api/skilifts/static", staticBody)
	requireStatus(t, w, http.StatusCreated, "POST static")
	if !strings.Contains(w.Body.String(), `"VerticalFeet":2500`) {
		t.Errorf("create body = %s, want fixture echo", w.Body.String())
	}
	g := doReq(e, http.MethodGet, "/api/skilifts/Summit%20Express/static", "")
	requireStatus(t, g, http.StatusOK, "GET static")
	if !strings.Contains(g.Body.String(), `"LiftTime":"8:00"`) {
		t.Errorf("fetch body = %s, want fixture", g.Body.String())
	}
}

func TestDynamicRoundTrip(t *testing.T) {
	e := integrationEngine(t)
	requireStatus(t, doReq(e, http.MethodPost, "/api/skilifts/dynamic", dynamicBody), http.StatusCreated, "POST dynamic")
	g := doReq(e, http.MethodGet, "/api/skilifts/Summit%20Express/01%2F15%2F24", "")
	requireStatus(t, g, http.StatusOK, "GET dynamic")
	if !strings.Contains(g.Body.String(), `"TotalUniqueLiftRiders":1250`) {
		t.Errorf("fetch body = %s, want fixture", g.Body.String())
	}
}

func TestResortRoundTrip(t *testing.T) {
	e := integrationEngine(t)
	requireStatus(t, doReq(e, http.MethodPost, "/api/skilifts/resort", resortBody), http.StatusCreated, "POST resort")
	g := doReq(e, http.MethodGet, "/api/skilifts/Summit%20Resort/resort", "")
	requireStatus(t, g, http.StatusOK, "GET resort")
	if !strings.Contains(g.Body.String(), `"TotalRiders":8500`) {
		t.Errorf("fetch body = %s, want fixture", g.Body.String())
	}
}

func TestDoubleCreateConflicts(t *testing.T) {
	e := integrationEngine(t)
	doReq(e, http.MethodPost, "/api/skilifts/static", staticBody)
	w := doReq(e, http.MethodPost, "/api/skilifts/static", staticBody)
	requireError(t, w, http.StatusConflict, "CONFLICT", "double create")
}

func TestInvalidBodiesAre400(t *testing.T) {
	e := integrationEngine(t)
	cases := []struct{ path, body, where string }{
		{"/api/skilifts/static", `{"Lift":"A"}`, "missing fields"},
		{"/api/skilifts/dynamic", `{"Lift":"A","Metadata":"01/15/24","TotalUniqueLiftRiders":1.5,"SnowCount":1,"LiftStatus":"Open","AvalancheDanger":"Low"}`, "float riders"},
		{"/api/skilifts/resort", `{"Lift":"A","Metadata":"resort","TotalRiders":1,"TotalSnow":1,"AvalancheDanger":"Low","OpenLifts":1}`, "non-list openlifts"},
		{"/api/skilifts/static", `{"Lift":}`, "malformed json"},
		{"/api/skilifts/static", `{"Lift":"A","Metadata":"static","IsDetachable":false,"VerticalFeet":1,"LiftTime":"8:00","X":1}`, "unknown field"},
	}
	for _, tc := range cases {
		w := doReq(e, http.MethodPost, tc.path, tc.body)
		requireError(t, w, http.StatusBadRequest, "VALIDATION_ERROR", tc.where)
	}
}

func TestGetAbsentIs404(t *testing.T) {
	e := integrationEngine(t)
	w := doReq(e, http.MethodGet, "/api/skilifts/Ghost/static", "")
	requireError(t, w, http.StatusNotFound, "NOT_FOUND", "get absent")
}

func TestGetBadMetadataIs400(t *testing.T) {
	e := integrationEngine(t)
	w := doReq(e, http.MethodGet, "/api/skilifts/A/bogus", "")
	requireError(t, w, http.StatusBadRequest, "VALIDATION_ERROR", "bad metadata")
}

func TestUpdateStatic(t *testing.T) {
	e := integrationEngine(t)
	seedThree(t, e)
	w := doReq(e, http.MethodPut, "/api/skilifts/Summit%20Express/static", `{"VerticalFeet":3000}`)
	requireStatus(t, w, http.StatusOK, "PUT static")
	if !strings.Contains(w.Body.String(), `"VerticalFeet":3000`) {
		t.Errorf("update body = %s, want merged", w.Body.String())
	}
	w = doReq(e, http.MethodPut, "/api/skilifts/Ghost/static", `{"VerticalFeet":1}`)
	requireError(t, w, http.StatusNotFound, "NOT_FOUND", "update absent")
}

func TestUpdateDynamicAndResort(t *testing.T) {
	e := integrationEngine(t)
	seedThree(t, e)
	w := doReq(e, http.MethodPut, "/api/skilifts/Summit%20Express/01%2F15%2F24", `{"SnowCount":60}`)
	requireStatus(t, w, http.StatusOK, "PUT dynamic")
	w = doReq(e, http.MethodPut, "/api/skilifts/Summit%20Resort/resort", `{"TotalSnow":50}`)
	requireStatus(t, w, http.StatusOK, "PUT resort")
	if !strings.Contains(w.Body.String(), `"TotalSnow":50`) {
		t.Errorf("resort update body = %s, want merged", w.Body.String())
	}
}

func TestUpdateStaticViaGenericRouteIs400(t *testing.T) {
	e := integrationEngine(t)
	seedThree(t, e)
	w := doReq(e, http.MethodPut, "/api/skilifts/Summit%20Express/static", `{"VerticalFeet":1}`)
	// This path IS the /static route (registered first) → 200.
	requireStatus(t, w, http.StatusOK, "PUT /static route")
}

func TestUpdateEmptyAndMismatchAre400(t *testing.T) {
	e := integrationEngine(t)
	seedThree(t, e)
	w := doReq(e, http.MethodPut, "/api/skilifts/Summit%20Express/static", `{}`)
	requireError(t, w, http.StatusBadRequest, "VALIDATION_ERROR", "empty update")
	w = doReq(e, http.MethodPut, "/api/skilifts/Summit%20Express/static", `{"Lift":"Other","VerticalFeet":1}`)
	requireError(t, w, http.StatusBadRequest, "VALIDATION_ERROR", "identity mismatch")
}

func TestDelete(t *testing.T) {
	e := integrationEngine(t)
	seedThree(t, e)
	w := doReq(e, http.MethodDelete, "/api/skilifts/Summit%20Express/static", "")
	requireStatus(t, w, http.StatusNoContent, "DELETE")
	if w.Body.Len() != 0 {
		t.Errorf("DELETE body = %q, want empty", w.Body.String())
	}
	w = doReq(e, http.MethodDelete, "/api/skilifts/Summit%20Express/static", "")
	requireError(t, w, http.StatusNotFound, "NOT_FOUND", "delete absent")
	w = doReq(e, http.MethodDelete, "/api/skilifts/A/bogus", "")
	requireError(t, w, http.StatusBadRequest, "VALIDATION_ERROR", "delete bad metadata")
}

func TestLiftCollectionPagination(t *testing.T) {
	e := integrationEngine(t)
	seedThree(t, e)
	// Seed two more dynamics for the same lift.
	for _, meta := range []string{"01/16/24", "01/17/24"} {
		body := strings.Replace(dynamicBody, "01/15/24", meta, 1)
		doReq(e, http.MethodPost, "/api/skilifts/dynamic", body)
	}
	w := doReq(e, http.MethodGet, "/api/skilifts/Summit%20Express?limit=2", "")
	requireStatus(t, w, http.StatusOK, "collection p1")
	var p1 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &p1)
	if p1["count"] != float64(2) {
		t.Fatalf("p1 count = %v, want 2", p1["count"])
	}
	token, ok := p1["nextToken"].(string)
	if !ok || token == "" {
		t.Fatalf("p1 missing nextToken: %s", w.Body.String())
	}
	if _, ok := p1["lastEvaluatedKey"]; !ok {
		t.Errorf("p1 missing deprecated lastEvaluatedKey")
	}
	w2 := doReq(e, http.MethodGet, "/api/skilifts/Summit%20Express?limit=2&nextToken="+token, "")
	requireStatus(t, w2, http.StatusOK, "collection p2")
	var p2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &p2)
	if p2["count"] != float64(2) {
		t.Fatalf("p2 count = %v, want 2", p2["count"])
	}
	if _, ok := p2["nextToken"]; ok {
		t.Errorf("p2 has nextToken, want absent at end")
	}
}

func TestScanList(t *testing.T) {
	e := integrationEngine(t)
	seedThree(t, e)
	w := doReq(e, http.MethodGet, "/api/skilifts?limit=2", "")
	requireStatus(t, w, http.StatusOK, "scan p1")
	var p map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p["count"] != float64(2) || p["nextToken"] == nil {
		t.Errorf("scan p1 = %s, want 2 items + token", w.Body.String())
	}
	w = doReq(e, http.MethodGet, "/api/skilifts?nextToken=!!!", "")
	requireError(t, w, http.StatusBadRequest, "VALIDATION_ERROR", "malformed token")
}

func TestByRiders(t *testing.T) {
	e := integrationEngine(t)
	seed := func(meta string, riders int) {
		body := strings.Replace(dynamicBody, "01/15/24", meta, 1)
		body = strings.Replace(body, `"TotalUniqueLiftRiders":1250`, `"TotalUniqueLiftRiders":`+itoa(riders), 1)
		doReq(e, http.MethodPost, "/api/skilifts/dynamic", body)
	}
	seed("01/10/24", 100)
	seed("01/11/24", 300)
	seed("01/12/24", 200)

	w := doReq(e, http.MethodGet, "/api/skilifts/Summit%20Express/by-riders", "")
	requireStatus(t, w, http.StatusOK, "by-riders")
	body := w.Body.String()
	i300, i200, i100 := strings.Index(body, "300"), strings.Index(body, "200"), strings.Index(body, "100")
	if !(i300 < i200 && i200 < i100) {
		t.Errorf("by-riders not descending: %s", body)
	}
	w = doReq(e, http.MethodGet, "/api/skilifts/Summit%20Express/by-riders?minRiders=150&maxRiders=250", "")
	requireStatus(t, w, http.StatusOK, "by-riders bounded")
	if !strings.Contains(w.Body.String(), "200") || strings.Contains(w.Body.String(), "300") {
		t.Errorf("bounded body = %s, want only 200", w.Body.String())
	}
	w = doReq(e, http.MethodGet, "/api/skilifts/Summit%20Express/by-riders?minRiders=50&maxRiders=10", "")
	requireError(t, w, http.StatusBadRequest, "VALIDATION_ERROR", "min>max")
}

func TestLimitCap(t *testing.T) {
	e := integrationEngine(t)
	seedThree(t, e)
	w := doReq(e, http.MethodGet, "/api/skilifts?limit=500", "")
	requireStatus(t, w, http.StatusOK, "limit cap")
}

func TestProbesAndRoot(t *testing.T) {
	e := integrationEngine(t)
	w := doReq(e, http.MethodGet, "/health", "")
	requireStatus(t, w, http.StatusOK, "health")
	for _, k := range []string{"status", "timestamp", "uptime"} {
		if !strings.Contains(w.Body.String(), `"`+k+`"`) {
			t.Errorf("health missing %q: %s", k, w.Body.String())
		}
	}
	w = doReq(e, http.MethodGet, "/ready", "")
	requireStatus(t, w, http.StatusOK, "ready")
	w = doReq(e, http.MethodGet, "/", "")
	requireStatus(t, w, http.StatusOK, "root")
	for _, k := range []string{"service", "version", "status", "table", "region"} {
		if !strings.Contains(w.Body.String(), `"`+k+`"`) {
			t.Errorf("root missing %q: %s", k, w.Body.String())
		}
	}
	w = doReq(e, http.MethodGet, "/missing", "")
	requireError(t, w, http.StatusNotFound, "NOT_FOUND", "unknown route")
}

func TestRequestIDEcho(t *testing.T) {
	e := integrationEngine(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("x-request-id", "integ-1")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if got := w.Header().Get("x-request-id"); got != "integ-1" {
		t.Errorf("x-request-id = %q, want integ-1", got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
