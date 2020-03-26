package eksawshelper

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws/arn"
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
