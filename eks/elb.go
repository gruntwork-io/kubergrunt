package eks

import (
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/retry"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/kubergrunt/commonerrors"
)

// waitForAnyInstancesRegisteredToALBOrNLB implements the logic to wait for instance registration to Application and
// Network Load Balancers. Refer to function docs for waitForAnyInstancesRegisteredToELB for more info.
// NOTE: this assumes the ELB is using the instance target type.
func waitForAnyInstancesRegisteredToALBOrNLB(logger *logrus.Entry, elbv2Svc *elbv2.ELBV2, lbName string, instanceIDsToWaitFor []string) error {
	targetGroups, err := getELBTargetGroups(elbv2Svc, lbName)
	if err != nil {
		return err
	}

	// Asynchronously wait for instances to be registered to each target group, collecting each goroutine error in
	// channels.
	wg := new(sync.WaitGroup)
	wg.Add(len(targetGroups))
	errChans := make(map[string]chan error, len(targetGroups))
	for _, targetGroup := range targetGroups {
		errChan := make(chan error, 1)
		errChans[aws.StringValue(targetGroup.TargetGroupName)] = errChan
		go asyncWaitForAnyInstancesRegisteredToTargetGroup(wg, errChan, logger, elbv2Svc, lbName, targetGroup, instanceIDsToWaitFor)
	}
	wg.Wait()

	// Collect all the errors from the async wait calls into a single error struct.
	var allErrs *multierror.Error
	for targetGroupName, errChan := range errChans {
		if err := <-errChan; err != nil {
			allErrs = multierror.Append(allErrs, err)
			logger.Errorf("Error waiting for instance to register to target group %s: %s", targetGroupName, err)
		}
	}
	finalErr := allErrs.ErrorOrNil()
	return errors.WithStackTrace(finalErr)
}

// asyncWaitForAnyInstancesRegisteredToTargetGroup waits for any instance to register to a single TargetGroup with
// retry. This function is intended to be run in a goroutine.
func asyncWaitForAnyInstancesRegisteredToTargetGroup(
	wg *sync.WaitGroup,
	errChan chan error,
	logger *logrus.Entry,
	elbv2Svc *elbv2.ELBV2,
	lbName string,
	targetGroup *elbv2.TargetGroup,
	instanceIDsToWaitFor []string,
) {
	defer wg.Done()

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
		errChan <- fatalWaitErr.Underlying
	}
	errChan <- waitErr
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

// getELBTargetGroups looks up the associated TargetGroup of the given ELB. Note that this assumes lbName refers to a v2
// ELB (ALB or NLB).
// NOTE: You can have multiple target groups on a given ELB if the service or ingress has multiple ports to listen on.
func getELBTargetGroups(elbv2Svc *elbv2.ELBV2, lbName string) ([]*elbv2.TargetGroup, error) {
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

	if len(targetGroupsResp.TargetGroups) == 0 {
		// This is an impossible condition because the load balancer controllers always creates at least 1 target group.
		return nil, errors.WithStackTrace(commonerrors.ImpossibleErr("ELB_HAS_UNEXPECTED_NUMBER_OF_TARGET_GROUPS"))
	}
	return targetGroupsResp.TargetGroups, nil
}
