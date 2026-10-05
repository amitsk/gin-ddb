package apperrors

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/gin-gonic/gin"
)

var errBoom = errors.New("boom")

func newConditionalCheckFailed() error {
	return &smithy.GenericAPIError{Code: "ConditionalCheckFailedException", Message: "condition failed"}
}

func newResourceNotFound() error {
	return &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "no table"}
}

func newThrottling(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: "slow down"}
}

func testContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request = req
	return c, w
}

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("response lacks error envelope: %s", w.Body.String())
	}
	return errObj
}

func TestWriteEnvelopeShape(t *testing.T) {
	c, w := testContext()
	Write(c, CodeValidationError, http.StatusBadRequest, "bad input",
		[]Detail{{Field: "Lift", Message: "required"}})

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	errObj := decodeEnvelope(t, w)
	if errObj["code"] != CodeValidationError {
		t.Errorf("code = %v, want %s", errObj["code"], CodeValidationError)
	}
	if errObj["message"] != "bad input" {
		t.Errorf("message = %v, want bad input", errObj["message"])
	}
	if errObj["statusCode"] != float64(400) {
		t.Errorf("statusCode = %v, want 400", errObj["statusCode"])
	}
	details, ok := errObj["details"].([]any)
	if !ok || len(details) != 1 {
		t.Fatalf("details = %v, want 1 entry", errObj["details"])
	}
}

func TestWriteEchoesIncomingRequestID(t *testing.T) {
	c, w := testContext()
	c.Request.Header.Set("x-request-id", "test-id-123")
	Write(c, CodeNotFound, http.StatusNotFound, "missing", nil)

	if got := w.Header().Get("x-request-id"); got != "test-id-123" {
		t.Errorf("x-request-id = %q, want incoming value echoed", got)
	}
}

func TestWriteGeneratesRequestIDWhenAbsent(t *testing.T) {
	c, w := testContext()
	Write(c, CodeInternalError, http.StatusInternalServerError, "Internal server error", nil)

	if got := w.Header().Get("x-request-id"); got == "" {
		t.Error("x-request-id empty, want generated UUID")
	}
}

func TestInternalErrorMessageLocked(t *testing.T) {
	if InternalErrorMessage != "Internal server error" {
		t.Errorf("InternalErrorMessage = %q, want exactly 'Internal server error'", InternalErrorMessage)
	}
}

func TestMapDynamoDBCreateConflict(t *testing.T) {
	code, status := MapDynamoDB(newConditionalCheckFailed(), true)
	if code != CodeConflict || status != http.StatusConflict {
		t.Errorf("create-path condition failure = (%s,%d), want (CONFLICT,409)", code, status)
	}
}

func TestMapDynamoDBUpdateDeleteNotFound(t *testing.T) {
	code, status := MapDynamoDB(newConditionalCheckFailed(), false)
	if code != CodeNotFound || status != http.StatusNotFound {
		t.Errorf("update-path condition failure = (%s,%d), want (NOT_FOUND,404)", code, status)
	}
}

func TestMapDynamoDBThrottling(t *testing.T) {
	for _, err := range []error{
		newThrottling("ThrottlingException"),
		newThrottling("ProvisionedThroughputExceededException"),
		newThrottling("RequestLimitExceeded"),
	} {
		code, status := MapDynamoDB(err, false)
		if code != CodeThrottled || status != http.StatusServiceUnavailable {
			t.Errorf("%v = (%s,%d), want (THROTTLED,503)", err, code, status)
		}
	}
}

func TestMapDynamoDBResourceNotFound(t *testing.T) {
	code, status := MapDynamoDB(newResourceNotFound(), false)
	if code != CodeNotFound || status != http.StatusNotFound {
		t.Errorf("missing table = (%s,%d), want (NOT_FOUND,404)", code, status)
	}
}

func TestMapDynamoDBGeneric(t *testing.T) {
	code, status := MapDynamoDB(errBoom, false)
	if code != CodeDynamoDBError || status != http.StatusInternalServerError {
		t.Errorf("generic failure = (%s,%d), want (DYNAMODB_ERROR,500)", code, status)
	}
}
