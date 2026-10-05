package config

import "testing"

func TestNonPositiveWindowRejected(t *testing.T) {
	for _, raw := range []string{"0s", "-1m"} {
		setEnv(t, "RATE_LIMIT_TIME_WINDOW", raw)
		if _, err := Load(); err == nil {
			t.Errorf("Load() with window %q = nil, want error", raw)
		}
	}
}
