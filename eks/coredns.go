package eks

import (
	"github.com/gruntwork-io/gruntwork-cli/errors"

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
	corednsAnnotation CorednsAnnotation,
) error {
	logger := logging.GetProjectLogger()

	switch corednsAnnotation {
	case Fargate:
		err := kubectl.RunKubectl(
			kubectlOptions,
			"patch", "deployment", "coredns",
			"-n", "kube-system",
			"--type", "json",
			"--patch", `[{
					op = "remove"
					path = "/spec/template/metadata/annotations/eks.amazonaws.com~1compute-type"
				}]`,
		)

		if err != nil {
			return errors.WithStackTrace(err)
		}
	case EC2:
		err := kubectl.RunKubectl(
			kubectlOptions,
			"patch", "deployment", "coredns",
			"-n", "kube-system",
			"--type", "json",
			"--patch", `[{
				op   = "add"
				path = "/spec/template/metadata/annotations"
				value = {
					"eks.amazonaws.com/compute-type" = "ec2"
				}
			}]`,
		)

		if err != nil {
			return errors.WithStackTrace(err)
		}
	}

	logger.Infof("Patched")
	return nil
}
