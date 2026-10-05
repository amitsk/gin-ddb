package apperrors

import (
	"net/http"
	"strings"
	"testing"
)

func TestNilDetailsOmitted(t *testing.T) {
	c, w := testContext()
	var noDetails []Detail
	Write(c, CodeNotFound, http.StatusNotFound, "item not found", noDetails)

	if strings.Contains(w.Body.String(), "details") {
		t.Errorf("body = %s, want no details key for typed-nil slice", w.Body.String())
	}
}
