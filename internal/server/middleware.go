// Package server assembles the Gin engine: middleware chain plus
// route wiring (handlers own the route logic). See spec §§4, 11.
package server

import (
	"compress/gzip"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gin-ddb/internal/apperrors"
	"gin-ddb/internal/config"

	"github.com/gin-gonic/gin"
)

// RequestIDMiddleware echoes the incoming x-request-id or mints one,
// on every response including errors.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := apperrors.RequestID(c)
		c.Header(apperrors.RequestIDHeader, id)
		c.Set(apperrors.RequestIDHeader, id)
		c.Next()
	}
}

// CORSMiddleware reflects the configured origin. When the origin is "*",
// credentials are forced off regardless of configuration (§11).
func CORSMiddleware(origin string, allowCreds bool) gin.HandlerFunc {
	if origin == "*" {
		allowCreds = false
	}
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		if allowCreds {
			h.Set("Access-Control-Allow-Credentials", "true")
		}
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// SecurityHeadersMiddleware sets nosniff everywhere and a default
// Content-Security-Policy on API routes (docs UI needs inline
// scripts, so /docs is exempt until the CSP value is locked in §15).
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		if !strings.HasPrefix(c.Request.URL.Path, "/docs") {
			c.Header("Content-Security-Policy", "default-src 'self'")
		}
		c.Next()
	}
}

// gzipBuffer captures the response to decide on compression.
type gzipBuffer struct {
	gin.ResponseWriter
	status int
	body   []byte
}

func (b *gzipBuffer) WriteHeader(status int) { b.status = status }

func (b *gzipBuffer) Write(data []byte) (int, error) {
	b.body = append(b.body, data...)
	return len(data), nil
}

// GzipMiddleware compresses responses at or above threshold bytes when
// the client sends Accept-Encoding: gzip.
func GzipMiddleware(threshold int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}
		buf := &gzipBuffer{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = buf
		c.Next()
		if len(buf.body) < threshold {
			buf.ResponseWriter.WriteHeader(buf.status)
			_, _ = buf.ResponseWriter.Write(buf.body)
			return
		}
		h := buf.ResponseWriter.Header()
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		buf.ResponseWriter.WriteHeader(buf.status)
		w := gzip.NewWriter(buf.ResponseWriter)
		_, _ = w.Write(buf.body)
		_ = w.Close()
	}
}

// rateBucket is one IP's fixed-window counter.
type rateBucket struct {
	count int
	start time.Time
}

// maxBuckets caps tracked IPs; beyond it, expired windows are swept.
const maxBuckets = 10000

// RateLimitMiddleware caps requests per IP per window. Disabled in test env.
func RateLimitMiddleware(max int, window time.Duration, disabled bool) gin.HandlerFunc {
	var mu sync.Mutex
	buckets := map[string]*rateBucket{}
	sweep := func(now time.Time) {
		for ip, b := range buckets {
			if now.Sub(b.start) >= 2*window {
				delete(buckets, ip)
			}
		}
	}
	return func(c *gin.Context) {
		if disabled {
			c.Next()
			return
		}
		ip := c.ClientIP()
		now := time.Now()
		mu.Lock()
		b, ok := buckets[ip]
		if !ok || now.Sub(b.start) >= window {
			if len(buckets) >= maxBuckets {
				sweep(now)
				if len(buckets) >= maxBuckets {
					// Still full of live windows: reset rather than grow
					// without bound (briefly lenient, never blocking).
					clear(buckets)
				}
			}
			b = &rateBucket{start: now}
			buckets[ip] = b
		}
		b.count++
		over := b.count > max
		mu.Unlock()
		if over {
			c.Header("Retry-After", "60")
			apperrors.Write(c, apperrors.CodeRateLimited,
				http.StatusTooManyRequests, "rate limit exceeded", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// LoggerMiddleware emits one structured line per request.
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString(apperrors.RequestIDHeader),
		)
	}
}

// New builds the engine with middleware but no routes (routes register
// separately so tests can mount subsets).
func NewBase(cfg *config.Config) *gin.Engine {
	if cfg.NodeEnv == "test" {
		gin.SetMode(gin.TestMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	e := gin.New()
	e.Use(gin.Recovery())
	e.Use(RequestIDMiddleware())
	e.Use(LoggerMiddleware())
	e.Use(CORSMiddleware(cfg.CORSOrigin, cfg.CORSAllowCredentials))
	e.Use(SecurityHeadersMiddleware())
	e.Use(GzipMiddleware(1024))
	e.Use(RateLimitMiddleware(cfg.RateLimitMax, cfg.RateLimitWindow, cfg.RateLimitDisabled()))
	return e
}
