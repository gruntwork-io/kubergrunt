package eksawshelper

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/eks"
)

// GetClusterNameFromArn extracts the EKS cluster name given the ARN for the cluster.
func GetClusterNameFromArn(eksClusterArnString string) (string, error) {
	eksClusterArn, err := arn.Parse(eksClusterArnString)
	if err != nil {
		return "", err
	}

	// EKS Cluster ARN resource section is cluster/CLUSTER_NAME, so we extract out the cluster name by droping the first
	// path.
	return strings.Join(strings.Split(eksClusterArn.Resource, "/")[1:], "/"), nil
}

// GetRegionFromArn extracts the AWS region that the EKS cluster is in from the ARN of the EKS cluster.
func GetRegionFromArn(eksClusterArnString string) (string, error) {
	eksClusterArn, err := arn.Parse(eksClusterArnString)
	if err != nil {
		return "", err
	}
	return eksClusterArn.Region, nil
}

// GetClusterArnByNameAndRegion looks up the EKS Cluster ARN using the region and EKS Cluster Name.
// For instances where we don't have the EKS Cluster ARN, such as within the Fargate Profile resource.
func GetClusterArnByNameAndRegion(eksClusterName string, region string) (string, error) {
	sess, err := NewAuthenticatedSession(region)
	if err != nil {
		return "", err
	}

	svc := eks.New(sess)
	input := &eks.DescribeClusterInput{
		Name: aws.String(eksClusterName),
	}

	output, err := svc.DescribeCluster(input)
	if err != nil {
		return "", err
	}
	return *output.Cluster.Arn, nil
}
