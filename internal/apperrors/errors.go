// Package apperrors owns error codes, the DynamoDB→HTTP mapping (§9),
// and the JSON envelope writer. It works on smithy API errors so the
// service layer stays free of DynamoDB SDK types.
package apperrors

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/aws/smithy-go"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Envelope codes per spec §9.
const (
	CodeNotFound        = "NOT_FOUND"
	CodeValidationError = "VALIDATION_ERROR"
	CodeConflict        = "CONFLICT"
	CodeThrottled       = "THROTTLED"
	CodeRateLimited     = "RATE_LIMITED"
	CodeDynamoDBError   = "DYNAMODB_ERROR"
	CodeInternalError   = "INTERNAL_ERROR"
	CodeRequestError    = "REQUEST_ERROR"
	CodeNotReady        = "NOT_READY"
)

// InternalErrorMessage is the only message an INTERNAL_ERROR ever carries.
const InternalErrorMessage = "Internal server error"

// RequestIDHeader is echoed on every response including errors.
const RequestIDHeader = "x-request-id"

// Detail is one entry of the VALIDATION_ERROR details array.
type Detail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// RequestID honors the incoming header, else generates a UUID.
func RequestID(c *gin.Context) string {
	if v := c.GetHeader(RequestIDHeader); v != "" {
		return v
	}
	return uuid.NewString()
}

// Write renders the {error:{message,code,statusCode,details?}} envelope.
// details is omitted when nil, including typed-nil slices and maps.
func Write(c *gin.Context, code string, status int, message string, details any) {
	c.Header(RequestIDHeader, RequestID(c))
	payload := gin.H{"message": message, "code": code, "statusCode": status}
	if !isNil(details) {
		payload["details"] = details
	}
	c.JSON(status, gin.H{"error": payload})
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

// Validation writes a 400 VALIDATION_ERROR with per-field details.
func Validation(c *gin.Context, message string, details []Detail) {
	Write(c, CodeValidationError, http.StatusBadRequest, message, details)
}

// MapDynamoDB maps a DynamoDB failure to (code, status).
// isCreate selects the create-path rule: failed exists-condition → 409,
// otherwise the same failure → 404.
func MapDynamoDB(err error, isCreate bool) (string, int) {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ConditionalCheckFailedException":
			if isCreate {
				return CodeConflict, http.StatusConflict
			}
			return CodeNotFound, http.StatusNotFound
		case "ResourceNotFoundException":
			return CodeNotFound, http.StatusNotFound
		case "ThrottlingException",
			"ProvisionedThroughputExceededException",
			"RequestLimitExceeded",
			"RequestLimitExceededException":
			return CodeThrottled, http.StatusServiceUnavailable
		}
	}
	return CodeDynamoDBError, http.StatusInternalServerError
}
