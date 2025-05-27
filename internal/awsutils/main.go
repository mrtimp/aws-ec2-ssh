package awsutils

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	log "github.com/sirupsen/logrus"
)

func LoadAwsDefaultConfig(profile string, region string) aws.Config {
	var optFns []func(options *config.LoadOptions) error

	if profile != "" {
		optFns = append(optFns, config.WithSharedConfigProfile(profile))
	}

	if region != "" {
		optFns = append(optFns, config.WithRegion(region))
	}

	log.Debugf("Loading AWS config: profile=%s, region=%s", profile, region)

	cfg, err := config.LoadDefaultConfig(context.Background(), optFns...)
	if err != nil {
		log.Fatal(err)
	}

	return cfg
}

func ResolveNameTagToInstanceID(name, profile, region string) (string, error) {
	cfg := LoadAwsDefaultConfig(profile, region)

	client := ec2.NewFromConfig(cfg)

	// filter running instances by Name tag
	out, err := client.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
			},
			{
				Name:   aws.String("tag:Name"),
				Values: []string{name},
			},
		},
	})
	if err != nil {
		log.Fatalf("Error describing instances: %v", err)
	}

	var matches []string
	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			matches = append(matches, aws.ToString(instance.InstanceId))
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("there are no running instances with Name tag: %s", name)
	}

	if len(matches) > 1 {
		return "", fmt.Errorf("there are multiple running instances with Name tag '%s': %v", name, matches)
	}

	return matches[0], nil
}
