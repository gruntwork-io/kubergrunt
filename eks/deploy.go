package eks

import (
	"math"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/gruntwork-io/go-commons/errors"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

type asgInfo struct {
	originalCapacity   int64
	maxSize            int64
	currentInstanceIDs []string
}

// RollOutDeployment will perform a zero downtime roll out of the current launch configuration associated with the
// provided ASG in the provided EKS cluster. This is accomplished by:
// 1. Double the desired capacity of the Auto Scaling Group that powers the EKS Cluster. This will launch new EKS
//    workers with the new launch configuration.
// 2. Wait for the new nodes to be ready for Pod scheduling in Kubernetes.
// 3. Cordon the old nodes so that no new Pods will be scheduled there.
// 4. Drain the pods scheduled on the old EKS workers (using the equivalent of "kubectl drain"), so that they will be
//    rescheduled on the new EKS workers.
// 5. Wait for all the pods to migrate off of the old EKS workers.
// 6. Set the desired capacity down to the original value and remove the old EKS workers from the ASG.
// TODO feature request: Break up into stages/checkpoints, and store state along the way so that command can pick up
// from a stage if something bad happens.
func RollOutDeployment(
	region string,
	eksAsgName string,
	kubectlOptions *kubectl.KubectlOptions,
	drainTimeout time.Duration,
	deleteLocalData bool,
	maxRetries int,
	sleepBetweenRetries time.Duration,
) (returnErr error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Beginning roll out for EKS cluster worker group %s in %s", eksAsgName, region)

	// Construct clients for AWS
	sess, err := eksawshelper.NewAuthenticatedSession(region)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	asgSvc := autoscaling.New(sess)
	ec2Svc := ec2.New(sess)
	elbSvc := elb.New(sess)
	elbv2Svc := elbv2.New(sess)
	logger.Infof("Successfully authenticated with AWS")

	// Retrieve the ASG object and gather required info we will need later
	asgInfo, err := getAsgInfo(asgSvc, eksAsgName)
	if err != nil {
		return err
	}

	// Calculate default max retries
	if maxRetries == 0 {
		maxRetries = getDefaultMaxRetries(asgInfo.originalCapacity, sleepBetweenRetries)
		logger.Infof(
			"No max retries set. Defaulted to %d based on sleep between retries duration of %s and scale up count %d.",
			maxRetries,
			sleepBetweenRetries,
			asgInfo.originalCapacity,
		)
	}

	// Make sure ASG is in steady state
	if asgInfo.originalCapacity != int64(len(asgInfo.currentInstanceIDs)) {
		logger.Infof("Ensuring ASG is in steady state (current capacity = desired capacity)")
		err = waitForCapacity(asgSvc, eksAsgName, maxRetries, sleepBetweenRetries)
		if err != nil {
			logger.Error("Error waiting for ASG to reach steady state. Try again after the ASG is in a steady state.")
			return err
		}
		logger.Infof("Verified ASG is in steady state (current capacity = desired capacity)")
		asgInfo, err = getAsgInfo(asgSvc, eksAsgName)
		if err != nil {
			return err
		}
	}

	// Make sure there is enough max size capacity to scale up
	maxCapacityForUpdate := asgInfo.originalCapacity * 2
	if asgInfo.maxSize < maxCapacityForUpdate {
		defer func() {
			err := setAsgMaxSize(asgSvc, eksAsgName, asgInfo.maxSize)
			if err != nil && returnErr == nil {
				returnErr = err
			}
		}()
		err := setAsgMaxSize(asgSvc, eksAsgName, maxCapacityForUpdate)
		if err != nil {
			return err
		}
	}

	logger.Infof("Starting with the following list of instances in ASG:")
	logger.Infof("%s", strings.Join(asgInfo.currentInstanceIDs, ","))

	logger.Infof("Launching new nodes with new launch config on ASG %s", eksAsgName)
	err = scaleUp(
		asgSvc,
		ec2Svc,
		elbSvc,
		elbv2Svc,
		kubectlOptions,
		eksAsgName,
		maxCapacityForUpdate,
		asgInfo.currentInstanceIDs,
		maxRetries,
		sleepBetweenRetries,
	)
	if err != nil {
		return err
	}
	logger.Infof("Successfully launched new nodes with new launch config on ASG %s", eksAsgName)

	logger.Infof("Cordoning old instances in cluster ASG %s to prevent Pod scheduling", eksAsgName)
	err = cordonNodesInAsg(ec2Svc, kubectlOptions, asgInfo.currentInstanceIDs)
	if err != nil {
		logger.Errorf("Error while cordoning nodes.")
		logger.Errorf("Continue to cordon nodes that failed manually, and then terminate the underlying instances to complete the rollout.")
		return err
	}
	logger.Infof("Successfully cordoned old instances in cluster ASG %s", eksAsgName)

	logger.Infof("Draining Pods on old instances in cluster ASG %s", eksAsgName)
	err = drainNodesInAsg(ec2Svc, kubectlOptions, asgInfo.currentInstanceIDs, drainTimeout, deleteLocalData)
	if err != nil {
		logger.Errorf("Error while draining nodes.")
		logger.Errorf("Continue to drain nodes that failed manually, and then terminate the underlying instances to complete the rollout.")
		return err
	}
	logger.Infof("Successfully drained all scheduled Pods on old instances in cluster ASG %s", eksAsgName)

	logger.Infof("Removing old nodes from ASG %s", eksAsgName)
	err = detachInstances(asgSvc, eksAsgName, asgInfo.currentInstanceIDs)
	if err != nil {
		logger.Errorf("Error while detaching the old instances.")
		logger.Errorf("Continue to detach the old instances and then terminate the underlying instances to complete the rollout.")
		return err
	}
	err = terminateInstances(ec2Svc, asgInfo.currentInstanceIDs)
	if err != nil {
		logger.Errorf("Error while terminating the old instances.")
		logger.Errorf("Continue to terminate the underlying instances to complete the rollout.")
		return err
	}
	logger.Infof("Successfully removed old nodes from ASG %s", eksAsgName)

	logger.Infof("Successfully finished roll out for EKS cluster worker group %s in %s", eksAsgName, region)
	return nil
}

// Retrieves current state of the ASG and returns the original Capacity and the IDs of the instances currently
// associated with it.
func getAsgInfo(asgSvc *autoscaling.AutoScaling, asgName string) (asgInfo, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Retrieving current ASG info")
	asg, err := GetAsgByName(asgSvc, asgName)
	if err != nil {
		return asgInfo{}, err
	}
	originalCapacity := *asg.DesiredCapacity
	maxSize := *asg.MaxSize
	currentInstances := asg.Instances
	currentInstanceIDs := idsFromAsgInstances(currentInstances)
	logger.Infof("Successfully retrieved current ASG info.")
	logger.Infof("\tCurrent desired capacity: %d", originalCapacity)
	logger.Infof("\tCurrent max size: %d", maxSize)
	logger.Infof("\tCurrent capacity: %d", len(currentInstances))
	return asgInfo{originalCapacity: originalCapacity, maxSize: maxSize, currentInstanceIDs: currentInstanceIDs}, nil
}

// Calculates the default max retries based on a heuristic of 5 minutes per wave. This assumes that the ASG scales up in
// waves of 10 instances, so the number of retries is:
// ceil(scaleUpCount / 10) * 5 minutes / sleepBetweenRetries
func getDefaultMaxRetries(scaleUpCount int64, sleepBetweenRetries time.Duration) int {
	logger := logging.GetProjectLogger()

	numWaves := int(math.Ceil(float64(scaleUpCount) / float64(10)))
	logger.Debugf("Calculated number of waves as %d (scaleUpCount %d)", numWaves, scaleUpCount)

	sleepBetweenRetriesSeconds := int(math.Trunc(sleepBetweenRetries.Seconds()))
	defaultMaxRetries := numWaves * 600 / sleepBetweenRetriesSeconds
	logger.Debugf(
		"Calculated default max retries as %d (scaleUpCount %d, num waves %d, duration (s) %d)",
		defaultMaxRetries,
		scaleUpCount,
		numWaves,
		sleepBetweenRetriesSeconds,
	)

	return defaultMaxRetries
}
