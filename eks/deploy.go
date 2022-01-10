package eks

import (
	"math"
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
// The process is broken up into stages/checkpoints, state is stored along the way so that command can pick up
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
	state, err := initDeployState(stateFile, ignoreRecoveryFile, maxRetries, sleepBetweenRetries)
	if err != nil {
		return err
	}

	err = state.gatherASGInfo(asgSvc, []string{eksAsgName})
	if err != nil {
		return err
	}

	err = state.setMaxCapacity(asgSvc)
	if err != nil {
		return err
	}

	err = state.scaleUp(asgSvc)
	if err != nil {
		return err
	}

	err = state.waitForNodes(ec2Svc, elbSvc, elbv2Svc, kubectlOptions)
	if err != nil {
		return err
	}

	err = state.cordonNodes(ec2Svc, kubectlOptions)
	if err != nil {
		return err
	}

	err = state.drainNodes(ec2Svc, kubectlOptions, drainTimeout, deleteLocalData)
	if err != nil {
		return err
	}

	err = state.detachInstances(asgSvc)
	if err != nil {
		return err
	}

	err = state.terminateInstances(ec2Svc)
	if err != nil {
		return err
	}

	err = state.restoreCapacity(asgSvc)
	if err != nil {
		return err
	}

	err = state.delete()
	if err != nil {
		logger.Warnf("Error deleting state file %s: %s", stateFile, err.Error())
		logger.Warn("Remove the file manually")
	}
	logger.Infof("Successfully finished roll out for EKS cluster worker group %s in %s", eksAsgName, region)
	return nil
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
