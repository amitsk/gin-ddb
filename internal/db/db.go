// Package db builds the DynamoDB client and owns table constants per §7.
package db

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"gin-ddb/internal/config"
)

// GSIFullName is the rider-ordered index: PK Lift (S) + SK TotalUniqueLiftRiders (N).
const GSIFullName = "SkiLiftsByRiders"

// TableName returns the configured table name.
func TableName(cfg *config.Config) string { return cfg.TableName }

// NewClient builds a DynamoDB client for local or AWS mode.
func NewClient(ctx context.Context, cfg *config.Config) (*dynamodb.Client, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if cfg.DynamoMode == "local" {
		region := cfg.LocalRegion
		if region == "" {
			region = cfg.Region
		}
		opts = append(opts,
			awsconfig.WithRegion(region),
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
		)
		if cfg.HasExplicitCreds() {
			opts = append(opts, awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
			))
		}
		if cfg.LocalEndpoint != "" {
			endpoint := cfg.LocalEndpoint
			opts = append(opts, awsconfig.WithEndpointResolverWithOptions(
				aws.EndpointResolverWithOptionsFunc(
					func(service, region string, options ...any) (aws.Endpoint, error) {
						return aws.Endpoint{URL: endpoint}, nil
					},
				),
			))
		}
	} else {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
		if cfg.HasExplicitCreds() {
			opts = append(opts, awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
			))
		}
	}
	awscfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return dynamodb.NewFromConfig(awscfg), nil
}
