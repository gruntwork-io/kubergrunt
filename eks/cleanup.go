package eks

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gruntwork-io/gruntwork-cli/errors"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/logging"
	"github.com/gruntwork-io/kubergrunt/retry"
)

// Set wait variables for NetworkInterface detaching and deleting
const waitSleepBetweenRetries time.Duration = 2 * time.Second
const waitMaxRetries int = 60

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
		// If it doesn't have an attachment, process the next network interface.
		if ni.Attachment == nil || aws.StringValue(ni.Attachment.Status) == "detached" {
			logger.Infof("Network interface %s is detached.", aws.StringValue(ni.NetworkInterfaceId))
			continue
		}

		err := requestDetach(ec2Svc, ni)

		switch {
		// Base case: no error means we can process the next interface.
		case err == nil:
			logger.Infof("Requested to detach network interface %s for security group %s", aws.StringValue(ni.NetworkInterfaceId), securityGroupID)
			continue
		// The attachment is already gone, so process the next network interface.
		case isNIAttachmentNotFoundErr(err):
			logger.Infof("Network interface %s is detached.", aws.StringValue(ni.NetworkInterfaceId))
			continue
		// Any other kind of error means we failed this cleanup.
		default:
			return errors.WithStackTrace(err)
		}
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

		err := retry.DoWithRetryE("Request Delete Network Interface", maxRetries, sleepBetweenRetries, func() error {
			_, err := ec2Svc.DeleteNetworkInterface(deleteNetworkInterfacesInput)

			if err == nil {
				logger.Infof("Requested to delete network interface %s for security group %s", aws.StringValue(ni.NetworkInterfaceId), securityGroupID)
				return nil
			}

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
				return nil // exit retry loop with success

			// Note: Handle InvalidParameterValue: Network interface [eni-id] is currently in use.
			// We suspect this is an issue with eventual consistency around AWS's resource state.
			case isAwsErr && awsErr.Code() == "InvalidParameterValue":
				logger.Infof("Waiting for network interface %s to not be in-use (eventual consistency issue).", aws.StringValue(ni.NetworkInterfaceId))
				return errors.WithStackTrace(err) // continue retrying

			default:
				logger.Errorf("Error requesting deleting network interface %s", aws.StringValue(ni.NetworkInterfaceId))
				return retry.FatalError{Underlying: err} // halt retries with error
			}
		})

		// All the retries failed or we hit a fatal error.
		if err != nil {
			return err
		}
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

	for _, ni := range networkInterfaces {
		logger.Infof("Waiting for network interface %s to reach detached state.", aws.StringValue(ni.NetworkInterfaceId))

		// Poll for the new status
		describeNetworkInterfacesInput := &ec2.DescribeNetworkInterfaceAttributeInput{
			Attribute:          aws.String("attachment"),
			NetworkInterfaceId: ni.NetworkInterfaceId,
		}

		err := retry.DoWithRetryE("Wait for Network Interface to be Detached", maxRetries, sleepBetweenRetries, func() error {
			niResult, err := ec2Svc.DescribeNetworkInterfaceAttribute(describeNetworkInterfacesInput)

			switch {
			// If there's no error, we have to keep trying.
			case err == nil:
				if niResult.Attachment != nil {
					logger.Warnf("Network interface %s attachment status: %s", aws.StringValue(ni.NetworkInterfaceId), aws.StringValue(niResult.Attachment.Status))
				}
				return errors.WithStackTrace(fmt.Errorf("Network Interface %s not detached.", aws.StringValue(ni.NetworkInterfaceId))) // continue retrying

			// Yay, we're detached, process the next network interface.
			case isNIDetachedErr(niResult, err) || isNINotFoundErr(err):
				logger.Infof("Network interface %s is detached.", aws.StringValue(ni.NetworkInterfaceId))
				return nil // exit retry loop with success

			default:
				logger.Errorf("Error polling attachment for network interface %s", aws.StringValue(ni.NetworkInterfaceId))
				return retry.FatalError{Underlying: err} // halt retries with error
			}
		})

		// All the retries failed or we hit a fatal error.
		if err != nil {
			if isMaxRetriesExceededErr(err) {
				return errors.WithStackTrace(NetworkInterfaceDetachedTimeoutError{aws.StringValue(ni.NetworkInterfaceId)})
			}
			return err
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

	for _, ni := range networkInterfaces {
		logger.Infof("Waiting for network interface %s to be deleted.", aws.StringValue(ni.NetworkInterfaceId))

		// Poll for the new status
		describeNetworkInterfacesInput := &ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: []*string{ni.NetworkInterfaceId},
		}

		err := retry.DoWithRetryE("Wait for Network Interface to be Deleted", maxRetries, sleepBetweenRetries, func() error {
			_, err := ec2Svc.DescribeNetworkInterfaces(describeNetworkInterfacesInput)

			switch {
			// If there's no error, we have to keep trying.
			case err == nil:
				return errors.WithStackTrace(fmt.Errorf("Network Interface %s not deleted.", aws.StringValue(ni.NetworkInterfaceId))) // continue retrying

			// Yay, it's deleted, process the next network interface.
			case isNINotFoundErr(err):
				logger.Infof("Network interface %s is deleted.", aws.StringValue(ni.NetworkInterfaceId))
				return nil // exit retry loop with success

			default:
				return retry.FatalError{Underlying: err} // halt retries with error
			}
		})

		// All the retries failed or we hit a fatal error.
		if err != nil {
			if isMaxRetriesExceededErr(err) {
				return errors.WithStackTrace(NetworkInterfaceDeletedTimeoutError{aws.StringValue(ni.NetworkInterfaceId)})
			}
			return err
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

func isNIAttachmentNotFoundErr(err error) bool {
	awsErr, isAwsErr := err.(awserr.Error)
	return isAwsErr && awsErr.Code() == "InvalidAttachmentID.NotFound"
}

func isNIDetachedErr(niResult *ec2.DescribeNetworkInterfaceAttributeOutput, err error) bool {
	return err == nil &&
		(niResult.Attachment == nil ||
			aws.StringValue(niResult.Attachment.Status) == "detached")
}

func isNINotFoundErr(err error) bool {
	awsErr, isAwsErr := err.(awserr.Error)
	return isAwsErr && awsErr.Code() == "InvalidNetworkInterfaceID.NotFound"
}

func isMaxRetriesExceededErr(err error) bool {
	_, isRetryErr := err.(retry.MaxRetriesExceeded)
	return isRetryErr
}

func requestDetach(
	ec2Svc *ec2.EC2,
	ni *ec2.NetworkInterface,
) error {
	detachInput := &ec2.DetachNetworkInterfaceInput{
		AttachmentId: aws.String(aws.StringValue(ni.Attachment.AttachmentId)),
	}
	_, err := ec2Svc.DetachNetworkInterface(detachInput)

	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}
