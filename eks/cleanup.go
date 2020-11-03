package eks

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gruntwork-io/gruntwork-cli/errors"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// CleanupSecurityGroup deletes the AWS EKS managed security group, which otherwise doesn't get cleaned up when
// destroying the EKS cluster.
// It also attempts to delete the security group left by ALB ingress controller.
func CleanupSecurityGroup(
	clusterArn string,
	securityGroupID string,
	vpcID string,
) error {
	logger := logging.GetProjectLogger()

	// Get Region from ARN
	region, err := eksawshelper.GetRegionFromArn(clusterArn)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// Get Cluster Name from ARN
	clusterID, err := eksawshelper.GetClusterNameFromArn(clusterArn)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// Start new AWS session
	sess, err := eksawshelper.NewAuthenticatedSession(region)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	ec2Svc := ec2.New(sess)
	logger.Infof("Successfully authenticated with AWS")

	logger.Infof("Deleting security group %s", securityGroupID)
	delSGInput := &ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(securityGroupID),
	}
	_, err = ec2Svc.DeleteSecurityGroup(delSGInput)
	if err != nil {
		// TODO?: check for Error.Code == 'InvalidGroup.NotFound'. This means it's already been deleted
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully deleted security group %s", securityGroupID)

	// Now delete ALB Ingress Controller's security group, if it exists

	logger.Infof("Looking up security group containing tag for EKS cluster %s", clusterID)
	//
	// 	input1 := &ec2.DescribeSecurityGroupsInput{
	// 		Filters: []*ec2.Filter{
	// 			{
	// 				Name:   aws.String("vpc-id"),
	// 				Values: []*string{aws.String(vpcID)},
	// 			},
	// 			{
	// 				Name:   aws.String("tag-key"),
	// 				Values: []*string{aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", clusterID))},
	// 			},
	// 		}}
	//
	sgInput := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
			{
				Name:   aws.String("tag:kubernetes.io/cluster-name"),
				Values: []*string{aws.String(clusterID)},
			},
		}}

	sgResult, err := ec2Svc.DescribeSecurityGroups(sgInput)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	logger.Infof("Found security group")
	//
	// 	results := append(result2.SecurityGroups, result1.SecurityGroups...)

	for _, result := range sgResult.SecurityGroups {
		groupID := *result.GroupId
		groupName := *result.GroupName
		input := &ec2.DeleteSecurityGroupInput{
			GroupId:   aws.String(groupID),
			GroupName: aws.String(groupName),
		}
		// DeleteSecurityGroup returns a struct with only private fields
		// so we can ignore the result.
		_, err := ec2Svc.DeleteSecurityGroup(input)
		if err != nil {
			// TODO?: check for Error.Code == 'InvalidGroup.NotFound'. This means it's already been deleted
			return errors.WithStackTrace(err)
		}

		logger.Infof("Successfully deleted security group with name=%s, id=%s", groupID, groupName)
	}

	return nil
}
