// Package config parses and validates process environment per spec §5.
// Invalid values fail fast with a joined "path: message" error.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the validated service environment.
type Config struct {
	NodeEnv              string
	Port                 int
	Host                 string
	LogLevel             string
	CORSOrigin           string
	CORSAllowCredentials bool
	RateLimitMax         int
	RateLimitWindow      time.Duration
	DynamoMode           string
	TableName            string
	LocalEndpoint        string
	LocalRegion          string
	Region               string
	AccessKeyID          string
	SecretAccessKey      string
}

// Load reads os.Environ and validates every variable.
func Load() (*Config, error) {
	return loadEnv(os.Getenv)
}

func loadEnv(get func(string) string) (*Config, error) {
	var bad []string
	fail := func(format string, args ...any) {
		bad = append(bad, fmt.Sprintf(format, args...))
	}

	port := 3000
	if v := strings.TrimSpace(get("PORT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			fail("PORT: must be an integer 1-65535")
		} else {
			port = n
		}
	}

	host := get("HOST")
	if strings.TrimSpace(host) == "" {
		host = "0.0.0.0"
	}

	logLevel := strings.ToLower(strings.TrimSpace(get("LOG_LEVEL")))
	if logLevel == "" {
		logLevel = "info"
	}
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		fail("LOG_LEVEL: must be one of debug|info|warn|error")
	}

	corsOrigin := get("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "*"
	}

	corsCreds := false
	if v := strings.TrimSpace(get("CORS_CREDENTIALS")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			fail("CORS_CREDENTIALS: must be a boolean")
		} else {
			corsCreds = b
		}
	}

	rateMax := 100
	if v := strings.TrimSpace(get("RATE_LIMIT_MAX")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			fail("RATE_LIMIT_MAX: must be an integer > 0")
		} else {
			rateMax = n
		}
	}

	rateWindow := time.Minute
	if v := strings.TrimSpace(get("RATE_LIMIT_TIME_WINDOW")); v != "" {
		d, err := parseWindow(v)
		if err != nil {
			fail("RATE_LIMIT_TIME_WINDOW: must be a duration like \"1 minute\" or \"1m\"")
		} else {
			rateWindow = d
		}
	}

	mode := strings.ToLower(strings.TrimSpace(get("DYNAMODB_MODE")))
	if mode == "" {
		mode = "local"
	}
	switch mode {
	case "local", "aws":
	default:
		fail("DYNAMODB_MODE: must be local|aws")
	}

	table := strings.TrimSpace(get("DYNAMODB_TABLE_NAME"))
	if table == "" {
		table = "SkiLifts"
	}

	endpoint := strings.TrimSpace(get("DYNAMODB_LOCAL_ENDPOINT"))
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		fail("DYNAMODB_LOCAL_ENDPOINT: must be a valid URL")
	}

	localRegion := strings.TrimSpace(get("DYNAMODB_LOCAL_REGION"))
	if localRegion == "" {
		localRegion = "us-east-1"
	}
	region := strings.TrimSpace(get("AWS_REGION"))
	if region == "" {
		region = "us-east-1"
	}

	keyID := get("AWS_ACCESS_KEY_ID")
	secret := get("AWS_SECRET_ACCESS_KEY")
	if (keyID == "") != (secret == "") {
		fail("AWS_ACCESS_KEY_ID: explicit creds require both key and secret")
	}

	if len(bad) > 0 {
		return nil, fmt.Errorf("invalid environment:\n%s", strings.Join(bad, "\n"))
	}

	return &Config{
		NodeEnv:              get("NODE_ENV"),
		Port:                 port,
		Host:                 host,
		LogLevel:             logLevel,
		CORSOrigin:           corsOrigin,
		CORSAllowCredentials: corsCreds,
		RateLimitMax:         rateMax,
		RateLimitWindow:      rateWindow,
		DynamoMode:           mode,
		TableName:            table,
		LocalEndpoint:        endpoint,
		LocalRegion:          localRegion,
		Region:               region,
		AccessKeyID:          keyID,
		SecretAccessKey:      secret,
	}, nil
}

// parseWindow accepts Go durations plus "<n> minute(s)/second(s)/hour(s)".
func parseWindow(v string) (time.Duration, error) {
	lower := strings.ToLower(strings.TrimSpace(v))
	replacer := strings.NewReplacer(
		"minutes", "m", "minute", "m",
		"seconds", "s", "second", "s",
		"hours", "h", "hour", "h",
		" ", "",
	)
	return time.ParseDuration(replacer.Replace(lower))
}

// RateLimitDisabled reports whether the IP rate limiter must be off.
func (c *Config) RateLimitDisabled() bool { return c.NodeEnv == "test" }

// HasExplicitCreds reports whether both static credentials are present.
func (c *Config) HasExplicitCreds() bool {
	return c.AccessKeyID != "" && c.SecretAccessKey != ""
}

// Addr returns the host:port listen address.
func (c *Config) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }
