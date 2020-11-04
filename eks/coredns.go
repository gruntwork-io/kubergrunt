package eks

import (
	"github.com/gruntwork-io/gruntwork-cli/errors"

	//"encoding/json"
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

		patch := `{
			"op": "remove",
			"path": "/spec/template/metadata/annotations/eks.amazonaws.com~1compute-type"
		}`

		// var raw map[string]interface{}

		// if err := json.Unmarshal(patch, &raw); err != nil {
		// 	return errors.WithStackTrace(err)
		// }
		// raw["count"] = 1

		// out, err := json.Marshal(raw)
		// if err != nil {
		// 	return errors.WithStackTrace(err)
		// }

		err = kubectl.RunKubectl(
			kubectlOptions,
			"patch", "deployment", "coredns",
			"-n", "kube-system",
			//"--patch", string(patch),
			"--patch", patch,
		)

		if err != nil {
			return errors.WithStackTrace(err)
		}
	case EC2:
		logger.Info("Doing ec2 annotation")

		patch := `{
			"op": "add",
			"path": "/spec/template/metadata/annotations",
			"value": {
				"eks.amazonaws.com/compute-type": "ec2"
			}
		}`

		// var raw map[string]interface{}

		// if err := json.Unmarshal(patch, &raw); err != nil {
		// 	return errors.WithStackTrace(err)
		// }
		// raw["count"] = 1

		// out, err := json.Marshal(raw)
		// if err != nil {
		// 	return errors.WithStackTrace(err)
		// }

		err = kubectl.RunKubectl(
			kubectlOptions,
			"patch", "deployment", "coredns",
			"-n", "kube-system",
			//"--patch", string(patch),
			"--patch", patch,
		)

		if err != nil {
			return errors.WithStackTrace(err)
		}
	}

	logger.Infof("Patched")
	return nil
}
