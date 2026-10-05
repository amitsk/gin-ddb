package db

import (
	"context"
	"testing"

	"gin-ddb/internal/config"
)

func localConfig() *config.Config {
	return &config.Config{
		DynamoMode: "local", TableName: "SkiLifts",
		LocalEndpoint: "http://localhost:8000", LocalRegion: "us-east-1",
		Region: "us-east-1",
	}
}

func TestTableName(t *testing.T) {
	if got := TableName(localConfig()); got != "SkiLifts" {
		t.Errorf("TableName() = %q, want SkiLifts", got)
	}
}

func TestNewClientLocal(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	client, err := NewClient(context.Background(), localConfig())
	if err != nil {
		t.Fatalf("NewClient(local) = %v, want nil", err)
	}
	if client == nil {
		t.Error("NewClient(local) = nil client, want non-nil")
	}
}
