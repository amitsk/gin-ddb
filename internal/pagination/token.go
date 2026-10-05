// Package pagination encodes DynamoDB LastEvaluatedKeys as opaque
// cursor tokens per spec §10: base64url (RawURLEncoding, no padding) of
// compact JSON with keys sorted alphabetically. Keys are raw string
// values ({Lift, Metadata}); the repository wraps them into
// AttributeValues so SDK types never leak into this layer.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// Encode renders a non-empty key map as an opaque cursor token.
func Encode(key map[string]string) (string, error) {
	if len(key) == 0 {
		return "", errors.New("pagination: empty key")
	}
	raw, err := json.Marshal(key) // encoding/json sorts map keys
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Decode parses a cursor token back into key values.
// Malformed tokens (bad base64, non-JSON, non-object, non-string
// values, empty object) all return an error for 400 mapping.
func Decode(token string) (map[string]string, error) {
	if token == "" {
		return nil, errors.New("pagination: empty token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.New("pagination: malformed token")
	}
	var key map[string]string
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, errors.New("pagination: malformed token")
	}
	if len(key) == 0 {
		return nil, errors.New("pagination: malformed token")
	}
	return key, nil
}
