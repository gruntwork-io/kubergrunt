package eks

import (
	"time"

	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// DrainASG will cordon and drain all the instances associated with the given ASGs at the time of running.
func DrainASG(
	region string,
	asgNames []string,
	kubectlOptions *kubectl.KubectlOptions,
	drainTimeout time.Duration,
	deleteEmptyDirData bool,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("All instances in the following worker groups will be drained:")
	for _, asgName := range asgNames {
		logger.Infof("\t- %s", asgName)
	}

	// Construct clients for AWS
	sess, err := eksawshelper.NewAuthenticatedSession(region)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	asgSvc := autoscaling.New(sess)
	ec2Svc := ec2.New(sess)
	logger.Infof("Successfully authenticated with AWS")

	// Retrieve instance IDs for each ASG requested.
	allInstanceIDs := []string{}
	for _, asgName := range asgNames {
		asgInfo, err := getAsgInfo(asgSvc, asgName)
		if err != nil {
			return err
		}
		allInstanceIDs = append(allInstanceIDs, asgInfo.OriginalInstances...)
	}
	logger.Infof("Found %d instances across all requested ASGs.", len(allInstanceIDs))

	// Cordon instances in the ASG to avoid scheduling evicted workloads on the instances being drained.
	logger.Info("Cordoning instances in requested ASGs.")
	if err := cordonNodesInAsg(ec2Svc, kubectlOptions, allInstanceIDs); err != nil {
		return err
	}
	logger.Info("Successfully cordoned all instances in requested ASGs.")

	// Now drain the pods from all the instances.
	logger.Info("Draining Pods scheduled on instances in requested ASGs.")
	if err := drainNodesInAsg(ec2Svc, kubectlOptions, allInstanceIDs, drainTimeout, deleteEmptyDirData); err != nil {
		return err
	}
	logger.Info("Successfully drained pods from all instances in requested ASGs.")

	return nil
}
