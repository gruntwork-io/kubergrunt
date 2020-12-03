package eks

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gruntwork-io/gruntwork-cli/errors"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// Set wait variables for NetworkInterface detaching and deleting
const waitSleepBetweenRetries time.Duration = 1 * time.Second
const waitMaxRetries int = 30

// CleanupSecurityGroup deletes the AWS EKS managed security group, which otherwise doesn't get cleaned up when
// destroying the EKS cluster. It also attempts to delete the security group left by ALB ingress controller, if applicable.
func CleanupSecurityGroup(
	clusterArn string,
	securityGroupID string,
	vpcID string,
) error {
	logger := logging.GetProjectLogger()

	region, err := eksawshelper.GetRegionFromArn(clusterArn)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	clusterID, err := eksawshelper.GetClusterNameFromArn(clusterArn)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	sess, err := eksawshelper.NewAuthenticatedSession(region)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	ec2Svc := ec2.New(sess)
	logger.Infof("Successfully authenticated with AWS")

	// 1. Delete main AWS-managed EKS security group
	err = deleteDependencies(ec2Svc, securityGroupID)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	logger.Infof("Deleting security group %s", securityGroupID)
	delSGInput := &ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(securityGroupID),
	}
	_, err = ec2Svc.DeleteSecurityGroup(delSGInput)
	if err != nil {
		if awsErr, isAwsErr := err.(awserr.Error); isAwsErr && awsErr.Code() == "InvalidGroup.NotFound" {
			logger.Infof("Security group %s already deleted.", securityGroupID)
			return nil
		}
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully deleted security group with name = %s", securityGroupID)

	// 2, Delete ALB Ingress Controller's security group, if it exists
	sgResult, err := lookupSecurityGroup(ec2Svc, vpcID, clusterID)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	for _, result := range sgResult.SecurityGroups {
		groupID := aws.StringValue(result.GroupId)
		groupName := aws.StringValue(result.GroupName)

		err = deleteDependencies(ec2Svc, groupID)
		if err != nil {
			return errors.WithStackTrace(err)
		}

		input := &ec2.DeleteSecurityGroupInput{
			GroupId:   aws.String(groupID),
			GroupName: aws.String(groupName),
		}
		_, err := ec2Svc.DeleteSecurityGroup(input)
		if err != nil {
			if awsErr, isAwsErr := err.(awserr.Error); isAwsErr && awsErr.Code() == "InvalidGroup.NotFound" {
				logger.Infof("Security group %s already deleted.", groupID)
				return nil
			}
			return errors.WithStackTrace(err)
		}

		logger.Infof("Successfully deleted security group with name=%s, id=%s", groupID, groupName)
	}

	return nil
}

// Detach and delete elastic network interfaces used by the security group
// so that the security group can be deleted.
func deleteDependencies(ec2Svc *ec2.EC2, securityGroupID string) error {
	networkInterfacesResult, err := findNetworkInterfaces(ec2Svc, securityGroupID)
	if err != nil {
		return err
	}

	err = detachNetworkInterfaces(ec2Svc, networkInterfacesResult, securityGroupID)
	if err != nil {
		return err
	}

	if len(networkInterfacesResult.NetworkInterfaces) > 0 {
		err = waitForNetworkInterfacesToBeDetached(ec2Svc, networkInterfacesResult.NetworkInterfaces, waitMaxRetries, waitSleepBetweenRetries)
		if err != nil {
			return err
		}
	}

	err = deleteNetworkInterfaces(ec2Svc, networkInterfacesResult, securityGroupID, waitMaxRetries, waitSleepBetweenRetries)
	if err != nil {
		return err
	}

	err = waitForNetworkInterfacesToBeDeleted(ec2Svc, networkInterfacesResult.NetworkInterfaces, waitMaxRetries, waitSleepBetweenRetries)
	if err != nil {
		return err
	}

	return nil
}

func findNetworkInterfaces(
	ec2Svc *ec2.EC2,
	securityGroupID string,
) (*ec2.DescribeNetworkInterfacesOutput, error) {
	logger := logging.GetProjectLogger()

	describeNetworkInterfacesInput := &ec2.DescribeNetworkInterfacesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("group-id"),
				Values: []*string{aws.String(securityGroupID)},
			},
		},
	}
	networkInterfacesResult, err := ec2Svc.DescribeNetworkInterfaces(describeNetworkInterfacesInput)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	for _, ni := range networkInterfacesResult.NetworkInterfaces {
		logger.Infof("Found network interface %s", aws.StringValue(ni.NetworkInterfaceId))
	}
	return networkInterfacesResult, nil
}

func detachNetworkInterfaces(
	ec2Svc *ec2.EC2,
	networkInterfaces *ec2.DescribeNetworkInterfacesOutput,
	securityGroupID string,
) error {
	logger := logging.GetProjectLogger()

	for _, ni := range networkInterfaces.NetworkInterfaces {
		// First check the network interface has an attachment. It might have gotten detached before we can even process it.
		// If it doesn't have an attachment, break to the next network interface.
		if ni.Attachment == nil || aws.StringValue(ni.Attachment.Status) == "detached" {
			logger.Infof("Network interface %s is detached.", aws.StringValue(ni.NetworkInterfaceId))
			break
		}

		detachInput := &ec2.DetachNetworkInterfaceInput{
			AttachmentId: aws.String(aws.StringValue(ni.Attachment.AttachmentId)),
		}
		_, err := ec2Svc.DetachNetworkInterface(detachInput)

		// If we have an error, check that it's NotFound. This is a success! Break to next iteration.
		if err != nil {
			if awsErr, isAwsErr := err.(awserr.Error); isAwsErr && awsErr.Code() == "InvalidAttachmentID.NotFound" {
				logger.Infof("Network interface %s is detached.", aws.StringValue(ni.NetworkInterfaceId))
				break
			}
			return errors.WithStackTrace(err)
		}
		logger.Infof("Requested to detach network interface %s for security group %s", aws.StringValue(ni.NetworkInterfaceId), securityGroupID)
	}
	return nil
}

func deleteNetworkInterfaces(
	ec2Svc *ec2.EC2,
	networkInterfaces *ec2.DescribeNetworkInterfacesOutput,
	securityGroupID string,
	maxRetries int,
	sleepBetweenRetries time.Duration,
) error {
	logger := logging.GetProjectLogger()

	for _, ni := range networkInterfaces.NetworkInterfaces {
		logger.Infof("Attempting to delete network interface %s", aws.StringValue(ni.NetworkInterfaceId))
		deleteNetworkInterfacesInput := &ec2.DeleteNetworkInterfaceInput{
			NetworkInterfaceId: ni.NetworkInterfaceId,
		}

		for i := 0; i < maxRetries; i++ {
			_, err := ec2Svc.DeleteNetworkInterface(deleteNetworkInterfacesInput)

			if err != nil {
				awsErr, isAwsErr := err.(awserr.Error)

				switch {
				// Note: Handle InvalidNetworkInterfaceID.NotFound error. We have a process, terraformVpcCniAwareDestroy, that
				// automatically cleans up detached ENIs. When that process runs, this loop will not be able to find those ENIs
				// anymore. But we're also thinking about removing that process in the future, because it might be obsolete now.
				// The process lives in terraform-aws-eks. If we handle the cleanup well in kubergrunt, we don't need that.
				// AWS might now be set to automatically delete detached ENIs, so it's doubly not needed, and we may even remove
				// the steps here to delete network interfaces and wait for their deletion.
				case isAwsErr && awsErr.Code() == "InvalidNetworkInterfaceID.NotFound":
					logger.Infof("Network interface %s is deleted.", aws.StringValue(ni.NetworkInterfaceId))
					i = maxRetries
					break // go to next NI

				// Note: Handle InvalidParameterValue: Network interface [eni-id] is currently in use.
				// We suspect this is an issue with eventual consistency around AWS's resource state.
				case isAwsErr && awsErr.Code() == "InvalidParameterValue":
					logger.Infof("Waiting for network interface %s to not be in-use (eventual consistency issue).", aws.StringValue(ni.NetworkInterfaceId))
					logger.Infof("Attempt %d of %d. Retrying after %s...", i+1, maxRetries, sleepBetweenRetries)
					time.Sleep(sleepBetweenRetries)
					break // go to next retry

				default:
					return errors.WithStackTrace(err)
				}
			}
		}
		logger.Infof("Requested to delete network interface %s for security group %s", aws.StringValue(ni.NetworkInterfaceId), securityGroupID)
	}

	return nil
}

func waitForNetworkInterfacesToBeDetached(
	ec2Svc *ec2.EC2,
	networkInterfaces []*ec2.NetworkInterface,
	maxRetries int,
	sleepBetweenRetries time.Duration,
) error {
	logger := logging.GetProjectLogger()
	countOfNetworkInterfaces := len(networkInterfaces)

	for _, ni := range networkInterfaces {
		logger.Infof("Waiting for network interface %s to reach detached state.", aws.StringValue(ni.NetworkInterfaceId))
		logger.Info("Checking network interface attachment status.")

		// Poll for the new status
		describeNetworkInterfacesInput := &ec2.DescribeNetworkInterfaceAttributeInput{
			Attribute:          aws.String("attachment"),
			NetworkInterfaceId: ni.NetworkInterfaceId,
		}

		for i := 0; i < maxRetries; i++ {
			niResult, err := ec2Svc.DescribeNetworkInterfaceAttribute(describeNetworkInterfacesInput)

			// If we have an error, check that it's NotFound. This is actually a success.
			if err != nil {
				if awsErr, isAwsErr := err.(awserr.Error); isAwsErr && awsErr.Code() == "InvalidNetworkInterfaceID.NotFound" {
					logger.Infof("Network interface %s is deleted.", aws.StringValue(ni.NetworkInterfaceId))

					// break out of all for loops
					countOfNetworkInterfaces = countOfNetworkInterfaces - 1
					if countOfNetworkInterfaces == 0 {
						logger.Info("All network interfaces are detached.")
						return nil
					}

					// break out of the retry for loop
					logger.Info("Checking next network interface.")
					break
				}

				logger.Errorf("Error polling network interface attribute: attachment for %s", aws.StringValue(ni.NetworkInterfaceId))
				return errors.WithStackTrace(err)
			}

			// If we're detached, go onto the next interface, or return.
			if niResult.Attachment == nil || aws.StringValue(niResult.Attachment.Status) == "detached" {
				logger.Infof("Network interface %s is detached.", aws.StringValue(ni.NetworkInterfaceId))

				// break out of all for loops
				countOfNetworkInterfaces = countOfNetworkInterfaces - 1
				if countOfNetworkInterfaces == 0 {
					logger.Info("All network interfaces are detached.")
					return nil
				}

				// break out of the retry for loop
				logger.Info("Checking next network interface.")
				break
			}

			if niResult.Attachment != nil {
				logger.Warnf("Network interface %s attachment status: %s", aws.StringValue(ni.NetworkInterfaceId), aws.StringValue(niResult.Attachment.Status))
			}

			// If we retried the max number of times to delete. Since it failed to cleanup, we exit with error.
			if i+1 >= maxRetries {
				return errors.WithStackTrace(NetworkInterfaceDetachedTimeoutError{aws.StringValue(ni.NetworkInterfaceId)})
			}

			logger.Infof("Attempt %d of %d. Retrying after %s...", i+1, maxRetries, sleepBetweenRetries)
			time.Sleep(sleepBetweenRetries)
		}
	}
	return nil
}

func waitForNetworkInterfacesToBeDeleted(
	ec2Svc *ec2.EC2,
	networkInterfaces []*ec2.NetworkInterface,
	maxRetries int,
	sleepBetweenRetries time.Duration,
) error {
	logger := logging.GetProjectLogger()
	countOfNetworkInterfaces := len(networkInterfaces)

	for _, ni := range networkInterfaces {
		logger.Infof("Waiting for network interface %s to be deleted.", aws.StringValue(ni.NetworkInterfaceId))
		logger.Info("Checking for network interface not found.")

		// Poll for the new status
		describeNetworkInterfacesInput := &ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: []*string{ni.NetworkInterfaceId},
		}

		for i := 0; i < maxRetries; i++ {
			_, err := ec2Svc.DescribeNetworkInterfaces(describeNetworkInterfacesInput)

			// If we have an error, check that it's NotFound. This is actually a success.
			if err != nil {
				if awsErr, isAwsErr := err.(awserr.Error); isAwsErr && awsErr.Code() == "InvalidNetworkInterfaceID.NotFound" {
					logger.Infof("Network interface %s is deleted.", aws.StringValue(ni.NetworkInterfaceId))

					// break out of all for loops
					countOfNetworkInterfaces = countOfNetworkInterfaces - 1
					if countOfNetworkInterfaces == 0 {
						logger.Info("All network interfaces are deleted.")
						return nil
					}

					// break out of the retry for loop
					logger.Info("Checking next network interface.")
					break
				}

				return errors.WithStackTrace(err)
			}

			// If we retried the max number of times to delete. Since it failed to cleanup, we exit with error.
			if i == maxRetries-1 {
				return errors.WithStackTrace(NetworkInterfaceDeletedTimeoutError{aws.StringValue(ni.NetworkInterfaceId)})
			}

			logger.Infof("Attempt %d of %d. Retrying after %s...", i+1, maxRetries, sleepBetweenRetries)
			time.Sleep(sleepBetweenRetries)
		}
	}
	return nil
}

// Used to look up the security group for the ALB ingress controller
func lookupSecurityGroup(
	ec2Svc *ec2.EC2,
	vpcID string,
	clusterID string,
) (*ec2.DescribeSecurityGroupsOutput, error) {
	logger := logging.GetProjectLogger()

	logger.Infof("Looking up security group containing tag for EKS cluster %s", clusterID)
	sgInput := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
			{
				// TODO: does this one still get created?
				//Name:   aws.String("tag:kubernetes.io/cluster-name"),
				// This one is created for sure:
				Name:   aws.String("tag:elbv2.k8s.aws/cluster"),
				Values: []*string{aws.String(clusterID)},
			},
		}}

	sgResult, err := ec2Svc.DescribeSecurityGroups(sgInput)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return sgResult, nil
}
