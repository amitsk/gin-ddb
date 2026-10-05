package handlers_test

import (
	"net/http"
	"testing"
)

func TestSlashShapedNonDateMetadataIs400(t *testing.T) {
	e := integrationEngine(t)
	for _, tc := range []struct{ method, path, where string }{
		{http.MethodGet, "/api/skilifts/A/ab%2Fcd%2Fef", "get non-date"},
		{http.MethodDelete, "/api/skilifts/A/ab%2Fcd%2Fef", "delete non-date"},
		{http.MethodPut, "/api/skilifts/A/ab%2Fcd%2Fef", "put non-date"},
	} {
		w := doReq(e, tc.method, tc.path, "")
		requireError(t, w, http.StatusBadRequest, "VALIDATION_ERROR", tc.where)
	}
}
