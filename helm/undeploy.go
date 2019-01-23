package helm

import (
	"fmt"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// Undeploy will undeploy (uninstall) the helm server and related Secrets from the Kubernetes cluster.
func Undeploy(
	kubectlOptions *kubectl.KubectlOptions,
	namespace string,
	helmHome string,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Undeploying Helm Server in namespace %s", namespace)

	logger.Info("Removing Helm Server")
	if err := helmReset(kubectlOptions, namespace, helmHome); err != nil {
		logger.Errorf("Error removing helm server: %s", err)
		return err
	}
	logger.Info("Successfully removed helm server")

	logger.Info("Removing Kubernetes Secrets holding TLS credentials")
	if err := removeHelmCredentials(kubectlOptions, namespace); err != nil {
		logger.Errorf("Error removing Kubernetes secrets: %s", err)
		return err
	}
	logger.Info("Successfully removed Kubernetes secrets holding TLS credentials")

	logger.Infof("Done undeploying Helm Server in namespace %s", namespace)
	return nil
}

// helmReset calls the reset subcommand in helm to uninstall the helm server from the Kubernetes cluster.
func helmReset(
	kubectlOptions *kubectl.KubectlOptions,
	namespace string,
	helmHome string,
) error {
	args := []string{
		"reset",
		"--tiller-namespace",
		namespace,
		"--tls",
		"--tls-verify",
	}
	if helmHome != "" {
		args = append(args, "--home")
		args = append(args, helmHome)
	}
	return RunHelm(kubectlOptions, args...)
}

// removeHelmCredentials will look up all the credentials created during a deploy, and remove them from the Kubernetes
// cluster.
func removeHelmCredentials(kubectlOptions *kubectl.KubectlOptions, namespace string) error {
	logger := logging.GetProjectLogger()

	secrets, err := kubectl.ListSecrets(
		kubectlOptions,
		namespace,
		metav1.ListOptions{
			LabelSelector: fmt.Sprintf("helm-namespace=%s,helm-server-credentials=true", namespace),
		},
	)
	if err != nil {
		return err
	}

	maybeErrors := MultiHelmError{Action: "removing credentials"}
	for _, secret := range secrets {
		err := kubectl.DeleteSecret(kubectlOptions, secret.Namespace, secret.Name)
		if err != nil {
			maybeErrors.AddError(err)
		}
	}
	if !maybeErrors.IsEmpty() {
		logger.Error("Error deleting credentials")
		return errors.WithStackTrace(maybeErrors)
	}
	return nil
}
