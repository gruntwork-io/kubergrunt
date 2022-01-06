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
	ignoreRecoveryFile bool,
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

	stateFile := defaultStateFile

	// Retrieve state if one exists or construct a new one
	state, err := readOrInitializeState(stateFile, ignoreRecoveryFile)

	// If we're in the initial state, gather ASG info and wait for capacity
	if !state.GatherASGInfoDone {
		// Retrieve the ASG object and gather required info we will need later
		tmpAsgInfo, err := getAsgInfo(asgSvc, eksAsgName)
		if err != nil {
			return err
		}

		// Calculate default max retries
		if maxRetries == 0 {
			maxRetries = getDefaultMaxRetries(tmpAsgInfo.originalCapacity, sleepBetweenRetries)
			logger.Infof(
				"No max retries set. Defaulted to %d based on sleep between retries duration of %s and scale up count %d.",
				maxRetries,
				sleepBetweenRetries,
				tmpAsgInfo.originalCapacity,
			)
		}

		// Make sure ASG is in steady state
		if tmpAsgInfo.originalCapacity != int64(len(tmpAsgInfo.currentInstanceIDs)) {
			logger.Infof("Ensuring ASG is in steady state (current capacity = desired capacity)")
			err = waitForCapacity(asgSvc, eksAsgName, maxRetries, sleepBetweenRetries)
			if err != nil {
				logger.Error("Error waiting for ASG to reach steady state. Try again after the ASG is in a steady state.")
				return err
			}
			logger.Infof("Verified ASG is in steady state (current capacity = desired capacity)")
			tmpAsgInfo, err = getAsgInfo(asgSvc, eksAsgName)
			if err != nil {
				return err
			}
		}

		state.GatherASGInfoDone = true
		state.ASG.MaxSize = tmpAsgInfo.maxSize
		state.ASG.Name = eksAsgName
		state.ASG.OriginalCapacity = tmpAsgInfo.originalCapacity
		state.ASG.OriginalInstances = tmpAsgInfo.currentInstanceIDs
		state.persist()
	}

	// Make sure there is enough max size capacity to scale up
	if !state.SetMaxCapacityDone {
		maxCapacityForUpdate := state.ASG.OriginalCapacity * 2
		if state.ASG.MaxSize < maxCapacityForUpdate {
			err := setAsgMaxSize(asgSvc, eksAsgName, maxCapacityForUpdate)
			if err != nil {
				return err
			}
		}
		state.ASG.MaxCapacityForUpdate = maxCapacityForUpdate
		state.SetMaxCapacityDone = true
		state.persist()
	}

	if !state.ScaleUpDone {
		logger.Infof("Starting with the following list of instances in ASG:")
		logger.Infof("%s", strings.Join(state.ASG.OriginalInstances, ","))

		logger.Infof("Launching new nodes with new launch config on ASG %s", state.ASG.Name)
		newInstanceIds, err := scaleUp(asgSvc, state.ASG.Name, state.ASG.OriginalInstances, state.ASG.MaxCapacityForUpdate, maxRetries, sleepBetweenRetries)
		if err != nil {
			return err
		}
		logger.Infof("Successfully launched new nodes with new launch config on ASG %s", eksAsgName)
		state.ScaleUpDone = true
		state.ASG.NewInstances = newInstanceIds
		state.persist()
	}

	if !state.WaitForNodesDone {
		err := waitAndVerifyNewInstances(ec2Svc, elbSvc, elbv2Svc, state.ASG.NewInstances, kubectlOptions, maxRetries, sleepBetweenRetries)
		if err != nil {
			logger.Errorf("Error while waiting for new nodes to be ready.")
			logger.Errorf("Either resume with the recovery file or terminate the new instances.")
			return err
		}
		state.WaitForNodesDone = true
		state.persist()
	}

	if !state.CordonNodesDone {
		logger.Infof("Cordoning old instances in cluster ASG %s to prevent Pod scheduling", eksAsgName)
		err = cordonNodesInAsg(ec2Svc, kubectlOptions, state.ASG.OriginalInstances)
		if err != nil {
			logger.Errorf("Error while cordoning nodes.")
			logger.Errorf("Either resume with the recovery file or continue to cordon nodes that failed manually, and then terminate the underlying instances to complete the rollout.")
			return err
		}
		logger.Infof("Successfully cordoned old instances in cluster ASG %s", eksAsgName)
		state.CordonNodesDone = true
		state.persist()
	}

	if !state.DrainNodesDone {
		logger.Infof("Draining Pods on old instances in cluster ASG %s", eksAsgName)
		err = drainNodesInAsg(ec2Svc, kubectlOptions, state.ASG.OriginalInstances, drainTimeout, deleteLocalData)
		if err != nil {
			logger.Errorf("Error while draining nodes.")
			logger.Errorf("Either resume with the recovery file or continue to drain nodes that failed manually, and then terminate the underlying instances to complete the rollout.")
			return err
		}
		logger.Infof("Successfully drained all scheduled Pods on old instances in cluster ASG %s", eksAsgName)
		state.DrainNodesDone = true
		state.persist()
	}

	if !state.DetachInstancesDone {
		logger.Infof("Removing old nodes from ASG %s: %s", eksAsgName, strings.Join(state.ASG.OriginalInstances, ","))
		err = detachInstances(asgSvc, eksAsgName, state.ASG.OriginalInstances)
		if err != nil {
			logger.Errorf("Error while detaching the old instances.")
			logger.Errorf("Either resume with the recovery file or continue to detach the old instances and then terminate the underlying instances to complete the rollout.")
			return err
		}
		state.DetachInstancesDone = true
		state.persist()
	}

	if !state.TerminateInstancesDone {
		logger.Infof("Terminating old nodes: %s", strings.Join(state.ASG.OriginalInstances, ","))
		err = terminateInstances(ec2Svc, state.ASG.OriginalInstances)
		if err != nil {
			logger.Errorf("Error while terminating the old instances.")
			logger.Errorf("Either resume with the recovery file or continue to terminate the underlying instances to complete the rollout.")
			return err
		}
		logger.Infof("Successfully removed old nodes from ASG %s", eksAsgName)
		state.TerminateInstancesDone = true
		state.persist()
	}

	if !state.RestoreCapacityDone {
		err := setAsgMaxSize(asgSvc, eksAsgName, state.ASG.MaxSize)
		if err != nil {
			logger.Errorf("Error while restoring ASG %s max size to %v.", state.ASG.Name, state.ASG.MaxSize)
			logger.Errorf("Either resume with the recovery file or adjust ASG max size manually to complete the rollout.")
			return err
		}
	}

	err = state.delete()
	if err != nil {
		logger.Errorf("Error deleting state file %s: %s", stateFile, err.Error())
	}
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
