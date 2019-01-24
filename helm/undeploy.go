package helm

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/collections"
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
	force bool,
	undeployReleases bool,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Undeploying Helm Server in namespace %s", namespace)

	if undeployReleases {
		logger.Warnf("Requested removal of all releases managed by Helm Server in namespace %s", namespace)
		err := deleteReleases(kubectlOptions, namespace, helmHome)
		if err != nil {
			logger.Errorf("Error attempting to remove all releases: %s", err)
			return err
		}
		logger.Warnf("Successfully removed all releases")
	}

	logger.Info("Removing Helm Server")
	if err := helmReset(kubectlOptions, namespace, helmHome, force); err != nil {
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
	force bool,
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
	if force {
		args = append(args, "--force")
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

// deleteReleases will delete all the Helm releases managed by the Helm Server in the provided namespace.
func deleteReleases(kubectlOptions *kubectl.KubectlOptions, namespace string, helmHome string) error {
	// First get a list of all the releases
	args := []string{
		"ls",
		"--short",
		"--tiller-namespace",
		namespace,
		"--tls",
		"--tls-verify",
	}
	if helmHome != "" {
		args = append(args, "--home")
		args = append(args, helmHome)
	}
	releasesRawString, err := RunHelmAndGetOutput(kubectlOptions, args...)
	if err != nil {
		return err
	}
	releases := strings.Split(releasesRawString, "\n")

	// Then, delete the releases in groups of 100
	for _, group := range collections.BatchListIntoGroupsOf(releases, 100) {
		deleteArgs := []string{
			"delete",
			"--tiller-namespace",
			namespace,
			"--tls",
			"--tls-verify",
		}
		deleteArgs = append(deleteArgs, group...)
		err := RunHelm(kubectlOptions, deleteArgs...)
		if err != nil {
			return err
		}
	}
	return nil
}
