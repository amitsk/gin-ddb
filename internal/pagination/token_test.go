package pagination

import (
	"encoding/base64"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	key := map[string]string{"Lift": "Summit Express", "Metadata": "01/15/24"}
	token, err := Encode(key)
	if err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	if token == "" {
		t.Fatal("Encode() returned empty token")
	}
	got, err := Decode(token)
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}
	if got["Lift"] != key["Lift"] || got["Metadata"] != key["Metadata"] {
		t.Errorf("round trip = %v, want %v", got, key)
	}
}

func TestEncodeSortsKeysAndOmitsPadding(t *testing.T) {
	token, err := Encode(map[string]string{"Metadata": "static", "Lift": "A"})
	if err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	for _, c := range token {
		if c == '=' || c == '+' || c == '/' {
			t.Errorf("token %q contains non-base64url char %q", token, c)
		}
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decodeRaw() = %v", err)
	}
	want := `{"Lift":"A","Metadata":"static"}`
	if string(raw) != want {
		t.Errorf("payload = %s, want sorted-keys compact %s", raw, want)
	}
}

func TestDecodedEquivalence(t *testing.T) {
	a, _ := Encode(map[string]string{"Lift": "A", "Metadata": "static"})
	// Same key bytes in different map order must decode equal.
	b, _ := Encode(map[string]string{"Metadata": "static", "Lift": "A"})
	da, errA := Decode(a)
	db, errB := Decode(b)
	if errA != nil || errB != nil {
		t.Fatalf("Decode() = %v, %v, want nil", errA, errB)
	}
	if da["Lift"] != db["Lift"] || da["Metadata"] != db["Metadata"] {
		t.Errorf("decoded tokens differ: %v vs %v", da, db)
	}
}

func TestMalformedTokens(t *testing.T) {
	for _, token := range []string{
		"",
		"!!!not-base64!!!",
		"e30",         // {} — valid base64url, not an object of strings... (empty object: rejected, no keys)
		"W10=",        // "[]" with padding
		"bm90LWpzb24", // "not-json"
	} {
		if _, err := Decode(token); err == nil {
			t.Errorf("Decode(%q) = nil, want error", token)
		}
	}
}

func TestEncodeEmptyKeyFails(t *testing.T) {
	if _, err := Encode(map[string]string{}); err == nil {
		t.Error("Encode(empty) = nil, want error")
	}
	if _, err := Encode(nil); err == nil {
		t.Error("Encode(nil) = nil, want error")
	}
}
