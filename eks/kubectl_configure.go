package eks

import (
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/gruntwork-io/gruntwork-cli/files"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// ConfigureKubectlForEks adds a new context to the kubeconfig located at the given path that can authenticate with the
// EKS cluster referenced by the given ARN.
func ConfigureKubectlForEks(
	eksCluster *eks.Cluster,
	kubectlOptions *kubectl.KubectlOptions,
) error {
	logger := logging.GetProjectLogger()

	// Load config from disk and then get actual data structure containing the parsed config information
	// Create a blank file if it does not exist already
	if !files.FileExists(kubectlOptions.ConfigPath) {
		if err := kubectl.CreateInitialConfig(kubectlOptions.ConfigPath); err != nil {
			return err
		}
	}
	logger.Infof("Loading kubectl config %s.", kubectlOptions.ConfigPath)
	kubeconfig := kubectl.LoadConfigFromPath(kubectlOptions.ConfigPath)
	rawConfig, err := kubeconfig.RawConfig()
	if err != nil {
		return err
	}
	logger.Infof("Successfully loaded and parsed kubectl config.")

	// Update the config data structure with the EKS cluster info
	err = kubectl.AddEksConfigContext(
		&rawConfig,
		kubectlOptions.ContextName,
		*eksCluster.Arn,
		*eksCluster.Name,
		*eksCluster.Endpoint,
		*eksCluster.CertificateAuthority.Data,
	)
	if err != nil {
		return err
	}

	// Update the current context to the newly created context
	logger.Infof("Setting current kubectl config context to %s.", kubectlOptions.ContextName)
	rawConfig.CurrentContext = kubectlOptions.ContextName
	logger.Info("Updated current kubectl config context.")

	// Finally, save the config to disk
	logger.Infof("Saving kubectl config updates to %s.", kubectlOptions.ConfigPath)
	err = clientcmd.ModifyConfig(kubeconfig.ConfigAccess(), rawConfig, false)
	if err != nil {
		return err
	}
	logger.Infof("Successfully saved kubectl config updates.")

	return nil
}
