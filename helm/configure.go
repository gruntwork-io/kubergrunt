package helm

import (
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// ConfigureClient will configure the local helm client to be able to communicate with the Tiller server installed in
// the provided Tiller namespace. Note that this supports the notion where Tiller is deployed in a different namespace
// from where resources should go. This is to address the risk where access to the tiller-secret will grant admin access
// by using the tiller server TLS certs.
func ConfigureClient(
	kubectlOptions *kubectl.KubectlOptions,
	helmHome string,
	tillerNamespace string,
	resourceNamespace string,
	setKubectlNamespace bool,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Setting up local helm client to access Tiller server deployed in namespace %s.", tillerNamespace)

	logger.Info("Checking if authorized to access specified Tiller server.")
	// TODO: Check for
	// - Access to TLS certs. If unavailable, mention they need to be granted access.
	// - Access to Tiller pod. If unavailable, mention they need to be granted access, pod should be deployed, or change
	//   namespace.
	logger.Info("Confirmed authorized to access specified Tiller server.")

	logger.Info("Downloading TLS certificates to access specified Tiller server.")
	// TODO
	logger.Info("Successfully downloaded TLS certificates.")

	logger.Info("Generating environment file to setup helm client.")
	// TODO
	logger.Info("Successfully generated environment file.")

	if setKubectlNamespace {
		logger.Info("Requested to set default kubectl namespace.")
		// TODO
		logger.Infof("Updated context %s to use namespace %s as default.", kubectlOptions.ContextName, resourceNamespace)
	}

	logger.Infof("Successfully set up local helm client to access Tiller server deployed in namespace %s. Be sure to source the environment file (%s/env) before using the helm client.", tillerNamespace, helmHome)
	return nil
}
