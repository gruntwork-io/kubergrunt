package eks

import (
	"strings"

	"github.com/gruntwork-io/go-commons/errors"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/kubectl"
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
	kubectlOptions *kubectl.KubectlOptions,
	clusterName string,
	fargateProfileArn string,
	corednsAnnotation CorednsAnnotation,
) error {
	logger := logging.GetProjectLogger()

	region, err := eksawshelper.GetRegionFromArn(fargateProfileArn)
	if err != nil {
		return err
	}
	logger.Infof("Got region %s", region)

	eksClusterArn, err := eksawshelper.GetClusterArnByNameAndRegion(clusterName, region)
	if err != nil {
		return err
	}
	logger.Infof("Got cluster arn %s", eksClusterArn)

	kubectlOptions.EKSClusterArn = eksClusterArn

	switch corednsAnnotation {
	case Fargate:
		logger.Info("Doing fargate annotation")

		// CoreDNS may or may not have the compute-type annotation by default. Check for existence.
		out, err := kubectl.RunKubectlWithOutput(
			kubectlOptions,
			"get", "deployment", "coredns",
			"-n", "kube-system",
			"-o", `jsonpath='{.spec.template.metadata}'`,
		)
		if err != nil {
			return errors.WithStackTrace(err)
		}

		// Only attempt to patch coredns deployment if the compute-type annotation is present.
		// Else trigger a update by executing a rollout. This is necessary for coredns to schedule.
		if strings.Contains(out, "compute-type") {
			err = kubectl.RunKubectl(
				kubectlOptions,
				"patch", "deployment", "coredns",
				"-n", "kube-system",
				"--type", "json",
				"--patch", `[{"op": "remove","path": "/spec/template/metadata/annotations/eks.amazonaws.com~1compute-type"}]`,
			)

			if err != nil {
				return errors.WithStackTrace(err)
			}
		} else {
			err = kubectl.RunKubectl(
				kubectlOptions,
				"rollout", "restart", "deployment", "coredns", "-n", "kube-system",
			)

			if err != nil {
				return errors.WithStackTrace(err)
			}
		}

	case EC2:
		logger.Info("Doing ec2 annotation")

		err = kubectl.RunKubectl(
			kubectlOptions,
			"patch", "deployment", "coredns",
			"-n", "kube-system",
			"--type", "json",
			"--patch", `[{"op": "add","path": "/spec/template/metadata/annotations","value": {"eks.amazonaws.com/compute-type": "ec2"}}]`,
		)

		if err != nil {
			return errors.WithStackTrace(err)
		}
	}

	logger.Infof("Patched")
	return nil
}
