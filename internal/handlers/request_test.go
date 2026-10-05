package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func serve(e *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestOversizeBodyIs413(t *testing.T) {
	e := integrationEngine(t)
	big := `{"Lift":"` + strings.Repeat("A", 2<<20) + `"}`
	w := doReq(e, http.MethodPost, "/api/skilifts/static", big)
	requireError(t, w, http.StatusRequestEntityTooLarge, "REQUEST_ERROR", "oversize body")
}

func TestNonJSONContentTypeIs415(t *testing.T) {
	e := integrationEngine(t)
	req := httptest.NewRequest(http.MethodPost, "/api/skilifts/static", strings.NewReader(staticBody))
	req.Header.Set("Content-Type", "text/plain")
	w := serve(e, req)
	requireError(t, w, http.StatusUnsupportedMediaType, "REQUEST_ERROR", "content type")
}
