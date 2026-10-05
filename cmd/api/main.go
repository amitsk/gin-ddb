// Command api runs the SkiLifts HTTP service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gin-ddb/internal/config"
	dbpkg "gin-ddb/internal/db"
	"gin-ddb/internal/repository"
	"gin-ddb/internal/server"
	"gin-ddb/internal/service"
)

// @title gin-ddb SkiLifts API
// @version 1.0.0
// @description Ski-lift records on DynamoDB: static, dynamic, and resort aggregates.
// @host localhost:3000
// @BasePath /
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return fmt.Errorf("LOG_LEVEL: %w", err)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	ctx := context.Background()
	client, err := dbpkg.NewClient(ctx, cfg)
	if err != nil {
		return fmt.Errorf("dynamodb client: %w", err)
	}
	svc := service.New(repository.New(client, cfg.TableName))
	engine := server.New(cfg, svc)

	srv := &http.Server{Addr: cfg.Addr(), Handler: engine}
	go func() {
		slog.Info("listening", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("draining in-flight requests")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
