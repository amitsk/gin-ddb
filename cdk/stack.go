// Package main defines the gin-ddb CDK stack per spec §12: table + GSI,
// VPC lookup, ECS Fargate service behind an ALB (/ready target check),
// autoscaling, alarms + SNS, and stack outputs.
package main

import (
	"os"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapplicationautoscaling"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awselasticloadbalancingv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

var _ = assertions.Template_FromStack

// containerEnv mirrors the §5 variable names. Secrets are included only
// when set in the synth environment — never baked in.
func containerEnv(tableName, region string) *map[string]*string {
	env := map[string]*string{
		"PORT":                      jsii.String("3000"),
		"HOST":                      jsii.String("0.0.0.0"),
		"LOG_LEVEL":                 jsii.String("info"),
		"CORS_ORIGIN":               jsii.String("*"),
		"CORS_CREDENTIALS":          jsii.String("false"),
		"RATE_LIMIT_MAX":            jsii.String("100"),
		"RATE_LIMIT_TIME_WINDOW":    jsii.String("1 minute"),
		"DYNAMODB_MODE":             jsii.String("aws"),
		"DYNAMODB_TABLE_NAME":       jsii.String(tableName),
		"DYNAMODB_LOCAL_REGION":     jsii.String("us-east-1"),
		"AWS_REGION":                jsii.String(region),
		"NODE_ENV":                  jsii.String("production"),
	}
	if key, secret := os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"); key != "" && secret != "" {
		env["AWS_ACCESS_KEY_ID"] = jsii.String(key)
		env["AWS_SECRET_ACCESS_KEY"] = jsii.String(secret)
	}
	return &env
}

// BuildResources creates every stack resource on an existing stack and
// VPC. Tests call this with an inline VPC; main passes a looked-up VPC.
func BuildResources(stack awscdk.Stack, vpc awsec2.IVpc, tableName string) {
	region := "us-east-1"
	if r := os.Getenv("AWS_REGION"); r != "" {
		region = r
	}

	table := awsdynamodb.NewTable(stack, jsii.String("SkiLifts"), &awsdynamodb.TableProps{
		TableName: jsii.String(tableName),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("Lift"), Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("Metadata"), Type: awsdynamodb.AttributeType_STRING,
		},
		BillingMode:   awsdynamodb.BillingMode_PAY_PER_REQUEST,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		PointInTimeRecoverySpecification: &awsdynamodb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: jsii.Bool(false),
		},
	})
	table.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
		IndexName: jsii.String("SkiLiftsByRiders"),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("Lift"), Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("TotalUniqueLiftRiders"), Type: awsdynamodb.AttributeType_NUMBER,
		},
		ProjectionType: awsdynamodb.ProjectionType_ALL,
	})

	cluster := awsecs.NewCluster(stack, jsii.String("Cluster"), &awsecs.ClusterProps{Vpc: vpc})

	logGroup := awslogs.NewLogGroup(stack, jsii.String("LogGroup"), &awslogs.LogGroupProps{
		Retention: awslogs.RetentionDays_ONE_MONTH,
	})

	taskDef := awsecs.NewFargateTaskDefinition(stack, jsii.String("TaskDef"), &awsecs.FargateTaskDefinitionProps{
		Cpu:            jsii.Number(256),
		MemoryLimitMiB: jsii.Number(512),
	})
	table.GrantReadWriteData(taskDef.TaskRole())
	taskDef.AddContainer(jsii.String("api"), &awsecs.ContainerDefinitionOptions{
		Image: awsecs.ContainerImage_FromRegistry(jsii.String("gin-ddb:latest"), nil),
		PortMappings: &[]*awsecs.PortMapping{
			{ContainerPort: jsii.Number(3000)},
		},
		Environment: containerEnv(tableName, region),
		Logging: awsecs.LogDriver_AwsLogs(&awsecs.AwsLogDriverProps{
			StreamPrefix: jsii.String("gin-ddb"),
			LogGroup:     logGroup,
		}),
		HealthCheck: &awsecs.HealthCheck{
			Command:  jsii.Strings("CMD-SHELL", "wget -q -O /dev/null http://localhost:3000/health || exit 1"),
			Interval: awscdk.Duration_Seconds(jsii.Number(30)),
			Retries:  jsii.Number(3),
			Timeout:  awscdk.Duration_Seconds(jsii.Number(5)),
		},
	})

	svc := awsecs.NewFargateService(stack, jsii.String("Service"), &awsecs.FargateServiceProps{
		Cluster:         cluster,
		TaskDefinition:  taskDef,
		DesiredCount:    jsii.Number(2),
		AssignPublicIp:  jsii.Bool(false),
	})

	alb := awselasticloadbalancingv2.NewApplicationLoadBalancer(stack, jsii.String("ALB"), &awselasticloadbalancingv2.ApplicationLoadBalancerProps{
		Vpc:            vpc,
		InternetFacing: jsii.Bool(true),
	})
	listener := alb.AddListener(jsii.String("Listener"), &awselasticloadbalancingv2.BaseApplicationListenerProps{
		Port: jsii.Number(80),
	})
	listener.AddTargets(jsii.String("Api"), &awselasticloadbalancingv2.AddApplicationTargetsProps{
		Port:     jsii.Number(3000),
		Protocol: awselasticloadbalancingv2.ApplicationProtocol_HTTP,
		Targets: &[]awselasticloadbalancingv2.IApplicationLoadBalancerTarget{svc},
		HealthCheck: &awselasticloadbalancingv2.HealthCheck{
			Path:              jsii.String("/ready"),
			HealthyHttpCodes:  jsii.String("200"),
		},
	})

	scaling := svc.AutoScaleTaskCount(&awsapplicationautoscaling.EnableScalingProps{
		MinCapacity: jsii.Number(1),
		MaxCapacity: jsii.Number(4),
	})
	scaling.ScaleOnCpuUtilization(jsii.String("CpuScaling"), &awsecs.CpuUtilizationScalingProps{
		TargetUtilizationPercent: jsii.Number(85),
	})

	alarms := awssns.NewTopic(stack, jsii.String("Alarms"), nil)
	cpuAlarm := awscloudwatch.NewAlarm(stack, jsii.String("CpuAlarm"), &awscloudwatch.AlarmProps{
		Metric:             svc.MetricCpuUtilization(nil),
		Threshold:          jsii.Number(85),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		EvaluationPeriods:  jsii.Number(2),
	})
	cpuAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(alarms))
	fivexx := awscloudwatch.NewAlarm(stack, jsii.String("FivexxAlarm"), &awscloudwatch.AlarmProps{
		Metric: alb.MetricHttpCodeTarget(
			awselasticloadbalancingv2.HttpCodeTarget_TARGET_5XX_COUNT,
			&awscloudwatch.MetricOptions{
				Period:    awscdk.Duration_Minutes(jsii.Number(1)),
				Statistic: jsii.String("Sum"),
			}),
		Threshold:          jsii.Number(10),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		EvaluationPeriods:  jsii.Number(1),
	})
	fivexx.AddAlarmAction(awscloudwatchactions.NewSnsAction(alarms))

	awscdk.NewCfnOutput(stack, jsii.String("TableName"), &awscdk.CfnOutputProps{
		Value: table.TableName(),
	})
	awscdk.NewCfnOutput(stack, jsii.String("ServiceURL"), &awscdk.CfnOutputProps{
		Value: alb.LoadBalancerDnsName(),
	})
}

func main() {
	defer jsii.Close()
	app := awscdk.NewApp(nil)
	var env *awscdk.Environment
	if acct, region := os.Getenv("CDK_DEFAULT_ACCOUNT"), os.Getenv("CDK_DEFAULT_REGION"); acct != "" && region != "" {
		env = &awscdk.Environment{Account: jsii.String(acct), Region: jsii.String(region)}
	}
	stack := awscdk.NewStack(app, jsii.String("GinDdbStack"), &awscdk.StackProps{Env: env})
	vpc := awsec2.Vpc_FromLookup(stack, jsii.String("Vpc"), &awsec2.VpcLookupOptions{
		IsDefault: jsii.Bool(true),
	})
	tableName := os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		tableName = "SkiLifts"
	}
	BuildResources(stack, vpc, tableName)
	app.Synth(nil)
}

var _ constructs.Construct
