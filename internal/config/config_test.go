package config

import (
	"testing"
	"time"
)

func setEnv(t *testing.T, k, v string) {
	t.Helper()
	t.Setenv(k, v)
}

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t,
		"NODE_ENV", "PORT", "HOST", "LOG_LEVEL", "CORS_ORIGIN",
		"CORS_CREDENTIALS", "RATE_LIMIT_MAX", "RATE_LIMIT_TIME_WINDOW",
		"DYNAMODB_MODE", "DYNAMODB_TABLE_NAME", "DYNAMODB_LOCAL_ENDPOINT",
		"DYNAMODB_LOCAL_REGION", "AWS_REGION", "AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
	)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with empty env = %v, want nil", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.CORSOrigin != "*" {
		t.Errorf("CORSOrigin = %q, want *", cfg.CORSOrigin)
	}
	if cfg.CORSAllowCredentials {
		t.Errorf("CORSAllowCredentials = true, want false")
	}
	if cfg.RateLimitMax != 100 {
		t.Errorf("RateLimitMax = %d, want 100", cfg.RateLimitMax)
	}
	if cfg.RateLimitWindow != time.Minute {
		t.Errorf("RateLimitWindow = %v, want 1m", cfg.RateLimitWindow)
	}
	if cfg.DynamoMode != "local" {
		t.Errorf("DynamoMode = %q, want local", cfg.DynamoMode)
	}
	if cfg.TableName != "SkiLifts" {
		t.Errorf("TableName = %q, want SkiLifts", cfg.TableName)
	}
	if cfg.LocalEndpoint != "http://localhost:8000" {
		t.Errorf("LocalEndpoint = %q, want http://localhost:8000", cfg.LocalEndpoint)
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("Region = %q, want us-east-1", cfg.Region)
	}
	if cfg.HasExplicitCreds() {
		t.Errorf("HasExplicitCreds() = true, want false with empty env")
	}
	if cfg.RateLimitDisabled() {
		t.Errorf("RateLimitDisabled() = true, want false outside test env")
	}
}

func TestLoadInvalidJoinsErrors(t *testing.T) {
	setEnv(t, "PORT", "abc")
	setEnv(t, "RATE_LIMIT_MAX", "-5")
	setEnv(t, "LOG_LEVEL", "verbose")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() with bad env = nil, want joined error")
	}
	msg := err.Error()
	for _, want := range []string{"PORT", "RATE_LIMIT_MAX", "LOG_LEVEL"} {
		if !contains(msg, want) {
			t.Errorf("error %q missing variable %q (want one line per bad variable)", msg, want)
		}
	}
}

func TestCredsBothOrNeither(t *testing.T) {
	setEnv(t, "AWS_ACCESS_KEY_ID", "key-only")
	clearEnv(t, "AWS_SECRET_ACCESS_KEY")
	if _, err := Load(); err == nil {
		t.Error("Load() with only access key = nil, want error")
	}
	setEnv(t, "AWS_ACCESS_KEY_ID", "key")
	setEnv(t, "AWS_SECRET_ACCESS_KEY", "secret")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with both creds = %v, want nil", err)
	}
	if !cfg.HasExplicitCreds() {
		t.Error("HasExplicitCreds() = false, want true")
	}
}

func TestRateLimitDisabledInTestEnv(t *testing.T) {
	setEnv(t, "NODE_ENV", "test")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if !cfg.RateLimitDisabled() {
		t.Error("RateLimitDisabled() = false with NODE_ENV=test, want true")
	}
}

func TestDynamoModeValidation(t *testing.T) {
	setEnv(t, "DYNAMODB_MODE", "embedded")
	if _, err := Load(); err == nil {
		t.Error("Load() with DYNAMODB_MODE=embedded = nil, want error")
	}
	setEnv(t, "DYNAMODB_MODE", "aws")
	if _, err := Load(); err != nil {
		t.Errorf("Load() with DYNAMODB_MODE=aws = %v, want nil", err)
	}
}

func TestRateLimitWindowParsing(t *testing.T) {
	setEnv(t, "RATE_LIMIT_TIME_WINDOW", "2 minutes")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if cfg.RateLimitWindow != 2*time.Minute {
		t.Errorf("RateLimitWindow = %v, want 2m", cfg.RateLimitWindow)
	}
	setEnv(t, "RATE_LIMIT_TIME_WINDOW", "90s")
	cfg, err = Load()
	if err != nil || cfg.RateLimitWindow != 90*time.Second {
		t.Errorf("Load() 90s = %+v, %v; want 90s", cfg, err)
	}
	setEnv(t, "RATE_LIMIT_TIME_WINDOW", "bogus")
	if _, err := Load(); err == nil {
		t.Error("Load() with bogus window = nil, want error")
	}
}

func TestAddr(t *testing.T) {
	clearEnv(t, "HOST", "PORT")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if cfg.Addr() != "0.0.0.0:3000" {
		t.Errorf("Addr() = %q, want 0.0.0.0:3000", cfg.Addr())
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
