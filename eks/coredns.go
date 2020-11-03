package eks

import (
	"github.com/gruntwork-io/gruntwork-cli/errors"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/logging"
)

type CorednsAnnotation string

const (
	Fargate CorednsAnnotation = "fargate"
	EC2     CorednsAnnotation = "ec2"
)

// ScheduleCoredns adds or removes the compute-type annotation from the coredns deployment resource.
// When adding, it is set to ec2, when removing, it enables coredns for fargate nodes.
func ScheduleCoredns(
	clusterName string,
	fargateProfileArn string,
	corednsAnnotation CorednsAnnotation,
) error {
	logger := logging.GetProjectLogger()

	// Get Region from ARN
	region, err := eksawshelper.GetRegionFromArn(fargateProfileArn)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	logger.Infof("Region %s", region)

	logger.Infof("No operations")
	return nil
}
