package main

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	awsec2 "github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/jsii-runtime-go"
)

func testTemplate(t *testing.T) assertions.Template {
	t.Helper()
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)
	vpc := awsec2.NewVpc(stack, jsii.String("Vpc"), &awsec2.VpcProps{MaxAzs: jsii.Number(2)})
	BuildResources(stack, vpc, "SkiLifts")
	return assertions.Template_FromStack(stack, nil)
}

func TestTableAndGSI(t *testing.T) {
	tmpl := testTemplate(t)
	tmpl.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]any{
		"TableName": "SkiLifts",
		"KeySchema": []any{
			map[string]any{"AttributeName": "Lift", "KeyType": "HASH"},
			map[string]any{"AttributeName": "Metadata", "KeyType": "RANGE"},
		},
		"GlobalSecondaryIndexes": []any{
			map[string]any{
				"IndexName": "SkiLiftsByRiders",
				"KeySchema": []any{
					map[string]any{"AttributeName": "Lift", "KeyType": "HASH"},
					map[string]any{"AttributeName": "TotalUniqueLiftRiders", "KeyType": "RANGE"},
				},
				"Projection": map[string]any{"ProjectionType": "ALL"},
			},
		},
	})
}

func TestServiceAndContainerEnv(t *testing.T) {
	tmpl := testTemplate(t)
	tmpl.ResourceCountIs(jsii.String("AWS::ECS::Service"), jsii.Number(1))
	tmpl.HasResourceProperties(jsii.String("AWS::ECS::TaskDefinition"), map[string]any{
		"ContainerDefinitions": assertions.Match_ArrayWith(&[]any{
			assertions.Match_ObjectLike(&map[string]any{
				"Environment": assertions.Match_ArrayWith(&[]any{
					assertions.Match_ObjectLike(&map[string]any{"Name": "DYNAMODB_TABLE_NAME"}),
					assertions.Match_ObjectLike(&map[string]any{"Name": "PORT"}),
				}),
				"HealthCheck": assertions.Match_ObjectLike(&map[string]any{
					"Command": assertions.Match_ArrayWith(&[]any{"CMD-SHELL"}),
				}),
			}),
		}),
	})
}

func TestALBHealthCheckOnReady(t *testing.T) {
	tmpl := testTemplate(t)
	tmpl.HasResourceProperties(jsii.String("AWS::ElasticLoadBalancingV2::TargetGroup"), map[string]any{
		"HealthCheckPath": "/ready",
	})
	tmpl.HasResourceProperties(jsii.String("AWS::ElasticLoadBalancingV2::Listener"), map[string]any{
		"Port":     80,
		"Protocol": "HTTP",
	})
}

func TestAutoscalingAlarmsAndOutputs(t *testing.T) {
	tmpl := testTemplate(t)
	tmpl.HasResourceProperties(jsii.String("AWS::ApplicationAutoScaling::ScalableTarget"), map[string]any{
		"MinCapacity": 1,
		"MaxCapacity": 4,
	})
	tmpl.HasResourceProperties(jsii.String("AWS::CloudWatch::Alarm"), map[string]any{
		"ComparisonOperator": "GreaterThanThreshold",
		"Threshold":          85,
	})
	tmpl.HasResourceProperties(jsii.String("AWS::SNS::Topic"), map[string]any{})
	outputs := tmpl.FindOutputs(jsii.String("*"), nil)
	found := map[string]bool{}
	for k := range *outputs {
		found[k] = true
	}
	if !found["TableName"] || !found["ServiceURL"] {
		t.Errorf("outputs = %v, want TableName + ServiceURL", found)
	}
}
