package eks

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/go-multierror"

	"github.com/gruntwork-io/kubergrunt/commonerrors"
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// GetAsgByName will lookup an AutoScalingGroup that matches the given name. This will return an error if it can not
// find any ASG that matches the given name.
func GetAsgByName(svc *autoscaling.AutoScaling, asgName string) (*autoscaling.Group, error) {
	input := autoscaling.DescribeAutoScalingGroupsInput{AutoScalingGroupNames: []*string{aws.String(asgName)}}
	output, err := svc.DescribeAutoScalingGroups(&input)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	groups := output.AutoScalingGroups
	if len(groups) == 0 {
		return nil, errors.WithStackTrace(NewLookupError("ASG", asgName, "detailed data"))
	}
	return groups[0], nil
}

// scaleUp will scale the ASG up and wait until all the nodes are available. Specifically:
// - Set the desired capacity on the ASG
// - Wait for the capacity in the ASG to meet the desired capacity (instances are launched)
// - Wait for the new instances to be ready in Kubernetes
// - Wait for the new instances to be registered with external load balancers
func scaleUp(
	asgSvc *autoscaling.AutoScaling,
	ec2Svc *ec2.EC2,
	elbSvc *elb.ELB,
	elbv2Svc *elbv2.ELBV2,
	kubectlOptions *kubectl.KubectlOptions,
	asgName string,
	desiredCapacity int64,
	oldInstanceIds []string,
	maxRetries int,
	sleepBetweenRetries time.Duration,
) error {
	logger := logging.GetProjectLogger()
	err := setAsgCapacity(asgSvc, asgName, desiredCapacity)
	if err != nil {
		logger.Errorf("Failed to set ASG capacity to %d", desiredCapacity)
		logger.Errorf("If the capacity is set in AWS, undo by lowering back to the original capacity. If the capacity is not yet set, triage the error message below and try again.")
		return err
	}
	err = waitForCapacity(asgSvc, asgName, maxRetries, sleepBetweenRetries)
	if err != nil {
		logger.Errorf("Timed out waiting for ASG to reach steady state.")
		// TODO: can we use stages to pick up from here?
		logger.Errorf("Undo by terminating all the new instances and trying again")
		return err
	}
	newInstanceIds, err := getLaunchedInstanceIds(asgSvc, asgName, oldInstanceIds)
	if err != nil {
		logger.Errorf("Error retrieving information about the ASG")
		// TODO: can we use stages to pick up from here?
		logger.Errorf("Undo by terminating all the new instances and trying again")
		return err
	}
	instances, err := instanceDetailsFromIds(ec2Svc, newInstanceIds)
	if err != nil {
		logger.Errorf("Error retrieving detailed about the instances")
		// TODO: can we use stages to pick up from here?
		logger.Errorf("Undo by terminating all the new instances and trying again")
		return err
	}
	eksKubeNodeNames := kubeNodeNamesFromInstances(instances)
	err = kubectl.WaitForNodesReady(
		kubectlOptions,
		eksKubeNodeNames,
		maxRetries,
		sleepBetweenRetries,
	)
	if err != nil {
		logger.Errorf("Timed out waiting for the instances to reach ready state in Kubernetes.")
		// TODO: can we use stages to pick up from here?
		logger.Errorf("Undo by terminating all the new instances and trying again")
		return err
	}
	elbs, err := kubectl.GetAWSLoadBalancers(kubectlOptions)
	if err != nil {
		logger.Errorf("Error retrieving associated ELB names of the Kubernetes services.")
		// TODO: can we use stages to pick up from here?
		logger.Errorf("Undo by terminating all the new instances and trying again")
		return err
	}
	err = waitForAnyInstancesRegisteredToELB(elbSvc, elbv2Svc, elbs, newInstanceIds)
	if err != nil {
		logger.Errorf("Timed out waiting for the instances to register to the Service ELBs.")
		// TODO: can we use stages to pick up from here?
		logger.Errorf("Undo by terminating all the new instances and trying again")
		return err
	}
	return err
}

// getLaunchedInstanceIds will return a list of instance IDs that are new in the ASG, given a list of IDs of the
// existing instances before any change was made.
func getLaunchedInstanceIds(svc *autoscaling.AutoScaling, asgName string, existingInstanceIds []string) ([]string, error) {
	asg, err := GetAsgByName(svc, asgName)
	if err != nil {
		return nil, err
	}
	allInstances := asg.Instances
	allInstanceIds := idsFromAsgInstances(allInstances)
	newInstanceIds := []string{}
	for _, inst := range allInstanceIds {
		if !collections.ListContainsElement(existingInstanceIds, inst) {
			newInstanceIds = append(newInstanceIds, inst)
		}
	}
	return newInstanceIds, nil
}

// setAsgCapacity will set the desired capacity on the auto scaling group. This will not wait for the ASG to expand or
// shrink to that size. See waitForCapacity to wait for the ASG to scale to the set capacity.
func setAsgCapacity(svc *autoscaling.AutoScaling, asgName string, desiredCapacity int64) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Updating ASG %s desired capacity to %d.", asgName, desiredCapacity)

	input := autoscaling.SetDesiredCapacityInput{
		AutoScalingGroupName: aws.String(asgName),
		DesiredCapacity:      aws.Int64(desiredCapacity),
	}
	_, err := svc.SetDesiredCapacity(&input)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	logger.Infof("Requested ASG %s desired capacity to be %d.", asgName, desiredCapacity)
	return nil
}

// waitForCapacity waits for the desired capacity to be reached
func waitForCapacity(
	svc *autoscaling.AutoScaling,
	asgName string,
	maxRetries int,
	sleepBetweenRetries time.Duration,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Waiting for ASG %s to reach desired capacity.", asgName)

	for i := 0; i < maxRetries; i++ {
		logger.Infof("Checking ASG %s capacity.", asgName)
		asg, err := GetAsgByName(svc, asgName)
		if err != nil {
			return err
		}
		currentCapacity := int64(len(asg.Instances))
		desiredCapacity := *asg.DesiredCapacity
		if currentCapacity == desiredCapacity {
			logger.Infof("ASG %s met desired capacity.", asgName)
			return nil
		}

		logger.Infof("ASG %s not yet at desired capacity %d (current %d).", asgName, desiredCapacity, currentCapacity)
		logger.Infof("Waiting for %s...", sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}
	return errors.WithStackTrace(
		NewCouldNotMeetASGCapacityError(
			asgName,
			"Timed out waiting for desired capacity to be reached.",
		),
	)
}

// idsFromAsgInstances takes a list of instance representations in ASG API, extract the instance ID (so we can fetch
// additional information later)
func idsFromAsgInstances(instances []*autoscaling.Instance) []string {
	idList := []string{}
	for _, inst := range instances {
		idList = append(idList, aws.StringValue(inst.InstanceId))
	}
	return idList
}

// Make the call to drain all the provided nodes in Kubernetes. This is different from terminating the instances:
// - Taint the nodes so that new pods are not scheduled
// - Evict all the pods gracefully
func drainNodesInAsg(
	ec2Svc *ec2.EC2,
	kubectlOptions *kubectl.KubectlOptions,
	asgInstanceIds []string,
	drainTimeout time.Duration,
	deleteLocalData bool,
) error {
	instances, err := instanceDetailsFromIds(ec2Svc, asgInstanceIds)
	if err != nil {
		return err
	}
	eksKubeNodeNames := kubeNodeNamesFromInstances(instances)

	return kubectl.DrainNodes(kubectlOptions, eksKubeNodeNames, drainTimeout, deleteLocalData)
}

// Make the call to cordon all the provided nodes in Kubernetes so that they won't be used to schedule new Pods.
func cordonNodesInAsg(
	ec2Svc *ec2.EC2,
	kubectlOptions *kubectl.KubectlOptions,
	asgInstanceIds []string,
) error {
	instances, err := instanceDetailsFromIds(ec2Svc, asgInstanceIds)
	if err != nil {
		return err
	}
	eksKubeNodeNames := kubeNodeNamesFromInstances(instances)

	return kubectl.CordonNodes(kubectlOptions, eksKubeNodeNames)
}

// detachInstances will request AWS to detach the instances, removing them from the ASG. In the process, it will also
// request to auto decrement the desired capacity.
func detachInstances(asgSvc *autoscaling.AutoScaling, asgName string, idList []string) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Detaching %d instances from ASG %s", len(idList), asgName)

	// AWS has a 20 instance limit for this, so we detach in groups of 20 ids
	for _, smallIDList := range collections.BatchListIntoGroupsOf(idList, 20) {
		input := &autoscaling.DetachInstancesInput{
			AutoScalingGroupName:           aws.String(asgName),
			InstanceIds:                    aws.StringSlice(smallIDList),
			ShouldDecrementDesiredCapacity: aws.Bool(true),
		}
		_, err := asgSvc.DetachInstances(input)
		if err != nil {
			return errors.WithStackTrace(err)
		}
	}

	logger.Infof("Detached %d instances from ASG %s", len(idList), asgName)
	return nil
}

// waitForAnyInstancesRegisteredToELB waits until any of the instances provided are registered to all the ELBs
// provided. Here we wait for any instance to be registered, because we only need one instance to be registered to
// preserve service uptime, due to the way Kubernetes works.
// Pros:
// - Shorter wait time.
// - Can continue on to drain nodes succinctly, which is also time consuming.
// - Scales better when there are many service load balancers.
// - Not a strict Pro but: more instances will continue to register, improving service up time as we go.
// Cons:
// - Not all instances are registered, so there is no "load balancing" initially. This may bring down the new server
//   that is launched.
// Ultimately, it was decided that the cons are not worth the extended wait time it will introduce to the command.
func waitForAnyInstancesRegisteredToELB(elbSvc *elb.ELB, elbv2Svc *elbv2.ELBV2, elbs []kubectl.AWSLoadBalancer, instanceIds []string) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Verifying new nodes are registered to external load balancers.")

	var multipleErrs *multierror.Error
	for _, elb := range elbs {
		if elb.TargetType == kubectl.IPTarget {
			// We ignore ELBs of the IP type as those directly link to Pods and not instances.
			continue
		} else if elb.TargetType == kubectl.UnknownELBTarget {
			// This should never happen, so we return a generic error that indicates this is an impossible condition and
			// almost 100% a bug with kubergrunt.
			multipleErrs = multierror.Append(commonerrors.ImpossibleErr("UNKNOWN_ELB_TARGET_TYPE_IN_WAIT"))
			continue
		}

		var err error
		switch elb.Type {
		case kubectl.CLB:
			err = waitForAnyInstancesRegisteredToCLB(logger, elbSvc, elb.Name, instanceIds)
		case kubectl.NLB, kubectl.ALB:
			err = waitForAnyInstancesRegisteredToALBOrNLB(logger, elbv2Svc, elb.Name, instanceIds)
		default:
			// This should never happen, so we return a generic error that indicates this is an impossible condition and
			// almost 100% a bug with kubergrunt.
			err = commonerrors.ImpossibleErr("UNKNOWN_ELB_TYPE_IN_WAIT")
		}
		if err != nil {
			multipleErrs = multierror.Append(multipleErrs, err)
		}
	}
	return multipleErrs.ErrorOrNil()
}
