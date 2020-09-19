package eksawshelper

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"

	"github.com/gruntwork-io/kubergrunt/logging"
)

// GetClusterByArn returns the EKS Cluster object that corresponds to the given ARN.
func GetClusterByArn(eksClusterArn string) (*eks.Cluster, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Retrieving details for EKS cluster %s", eksClusterArn)

	region, err := GetRegionFromArn(eksClusterArn)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	logger.Infof("Detected cluster deployed in region %s", region)

	client, err := NewEksClient(region)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	eksClusterName, err := GetClusterNameFromArn(eksClusterArn)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	describeClusterOutput, err := client.DescribeCluster(&eks.DescribeClusterInput{Name: aws.String(eksClusterName)})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	logger.Infof("Successfully retrieved EKS cluster details")

	return describeClusterOutput.Cluster, nil
}

func GetKubernetesTokenForCluster(clusterID string) (*token.Token, string, error) {
	gen, err := token.NewGenerator(false, false)
	if err != nil {
		return nil, "", errors.WithStackTrace(err)
	}
	tok, err := gen.Get(clusterID)
	return &tok, gen.FormatJSON(tok), errors.WithStackTrace(err)
}

// NewEksClient creates an EKS client.
func NewEksClient(region string) (*eks.EKS, error) {
	sess, err := NewAuthenticatedSession(region)
	if err != nil {
		return nil, err
	}
	return eks.New(sess), nil
}
