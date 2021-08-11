package eks

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/retry"
	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/kubergrunt/commonerrors"
)

// waitForAnyInstancesRegisteredToALBOrNLB implements the logic to wait for instance registration to Application and
// Network Load Balancers. Refer to function docs for waitForAnyInstancesRegisteredToELB for more info.
// NOTE: this assumes the ELB is using the instance target type.
func waitForAnyInstancesRegisteredToALBOrNLB(logger *logrus.Entry, elbv2Svc *elbv2.ELBV2, lbName string, instanceIDsToWaitFor []string) error {
	targetGroup, err := getELBTargetGroup(elbv2Svc, lbName)
	if err != nil {
		return err
	}

	// Retry up to 10 minutes with 15 second retry sleep
	waitErr := retry.DoWithRetry(
		logger.Logger,
		fmt.Sprintf(
			"wait for expected targets to be registered to target group %s of load balancer %s",
			aws.StringValue(targetGroup.TargetGroupName),
			lbName,
		),
		40, 15*time.Second,
		func() error {
			targetsResp, err := elbv2Svc.DescribeTargetHealth(&elbv2.DescribeTargetHealthInput{TargetGroupArn: targetGroup.TargetGroupArn})
			if err != nil {
				return retry.FatalError{Underlying: err}
			}

			// Check each target to see if it is one of the instances we are waiting for, and return without error to
			// stop the retry loop if that is the case since condition is met.
			for _, targetHealth := range targetsResp.TargetHealthDescriptions {
				if targetHealth.Target == nil || targetHealth.Target.Id == nil {
					continue
				}
				instanceID := *targetHealth.Target.Id
				if collections.ListContainsElement(instanceIDsToWaitFor, instanceID) {
					return nil
				}
			}
			return fmt.Errorf("No expected instances registered yet")
		},
	)
	if fatalWaitErr, isFatalErr := waitErr.(retry.FatalError); isFatalErr {
		return errors.WithStackTrace(fatalWaitErr.Underlying)
	}
	return errors.WithStackTrace(waitErr)
}

// waitForAnyInstancesRegisteredToCLB implements the logic to wait for instance registration to Classic Load Balancers.
// Refer to function docs for waitForAnyInstancesRegisteredToELB for more info.
func waitForAnyInstancesRegisteredToCLB(logger *logrus.Entry, elbSvc *elb.ELB, lbName string, instanceIds []string) error {
	instances := []*elb.Instance{}
	for _, instanceID := range instanceIds {
		instances = append(instances, &elb.Instance{InstanceId: aws.String(instanceID)})
	}

	logger.Infof("Waiting for at least one instance to be in service for elb %s", lbName)
	params := &elb.DescribeInstanceHealthInput{
		LoadBalancerName: aws.String(lbName),
		Instances:        instances,
	}
	err := elbSvc.WaitUntilAnyInstanceInService(params)
	if err != nil {
		logger.Errorf("error waiting for any instance to be in service for elb %s", lbName)
		return err
	}
	logger.Infof("At least one instance in service for elb %s", lbName)
	return nil
}

// getELBTargetGroup looks up the associated TargetGroup of the given ELB. Note that this assumes:
// - lbName refers to a v2 ELB (ALB or NLB)
// - There is exactly one TargetGroup associated with the ELB (this is enforced by the Kubernetes controllers)
func getELBTargetGroup(elbv2Svc *elbv2.ELBV2, lbName string) (*elbv2.TargetGroup, error) {
	resp, err := elbv2Svc.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{Names: aws.StringSlice([]string{lbName})})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if len(resp.LoadBalancers) == 0 {
		return nil, errors.WithStackTrace(CouldNotFindLoadBalancerErr{name: lbName})
	} else if len(resp.LoadBalancers) > 1 {
		// This condition is impossible because we are querying a single LB name and names are unique within regions.
		return nil, errors.WithStackTrace(commonerrors.ImpossibleErr("MORE_THAN_ONE_ELB_IN_LOOKUP"))
	} else if resp.LoadBalancers[0] == nil {
		return nil, errors.WithStackTrace(commonerrors.ImpossibleErr("ELB_IS_NULL_FROM_API"))
	}
	elb := resp.LoadBalancers[0]

	targetGroupsResp, err := elbv2Svc.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{LoadBalancerArn: elb.LoadBalancerArn})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if len(targetGroupsResp.TargetGroups) != 1 {
		// This is an impossible condition because the load balancer controllers always only creates a single target
		// group for the ELBs it provisions.
		return nil, errors.WithStackTrace(commonerrors.ImpossibleErr("ELB_HAS_UNEXPECTED_NUMBER_OF_TARGET_GROUPS"))
	}
	return targetGroupsResp.TargetGroups[0], nil
}
