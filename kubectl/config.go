package kubectl

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	homedir "github.com/mitchellh/go-homedir"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/gruntwork-io/package-k8s/modules/kubergrunt/logging"
)

// This will create an initial blank config
func CreateInitialConfig(kubeconfigPath string) error {
	parentDir := filepath.Dir(kubeconfigPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return errors.WithStackTrace(err)
	}

	if err := ioutil.WriteFile(kubeconfigPath, []byte(INITIAL_BLANK_KUBECONFIG), 0644); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// AddEksConfigContext will add the EKS cluster authentication info as a new context in the kubectl config. This will
// update the config object in place, adding in the:
// - cluster entry with the CA and endpoint information
// - auth info entry with execution settings to retrieve token via IAM
// - context entry to link the cluster and authinfo entries
func AddEksConfigContext(
	config *api.Config,
	contextName string,
	eksClusterArnString string,
	eksClusterName string,
	eksEndpoint string,
	b64CertificateAuthorityData string,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Adding new kubectl config context %s for authenticating with EKS cluster %s", contextName, eksClusterName)

	_, ok := config.Contexts[contextName]
	if ok {
		return errors.WithStackTrace(NewContextAlreadyExistsError(contextName))
	}

	// Insert new cluster to config
	err := AddClusterToConfig(
		config,
		eksClusterArnString,
		eksEndpoint,
		b64CertificateAuthorityData,
	)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// Insert auth info to config
	err = AddEksAuthInfoToConfig(config, eksClusterArnString, eksClusterName)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// Finally, insert the context
	return AddContextToConfig(config, contextName, eksClusterArnString, eksClusterArnString)
}

// AddClusterToConfig will append a new cluster to the kubectl config, based on its endpoint and certificate authority
// data.
func AddClusterToConfig(
	config *api.Config,
	name string,
	endpoint string,
	b64CertificateAuthorityData string,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Appending cluster info for %s to kubectl config.", name)
	cluster := api.NewCluster()
	certificateAuthorityData, err := base64.StdEncoding.DecodeString(b64CertificateAuthorityData)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	cluster.CertificateAuthorityData = certificateAuthorityData
	cluster.Server = endpoint
	config.Clusters[name] = cluster
	logger.Infof("Successfully appended cluster info.")
	return nil
}

// AddEksAuthInfoToConfig will add an exec command based AuthInfo entry to the kubectl config that is designed to
// retrieve the Kubernetes auth token using AWS IAM credentials. This will use the `token` command provided by
// `kubergrunt`.
func AddEksAuthInfoToConfig(config *api.Config, eksClusterArnString string, eksClusterName string) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Appending EKS cluster authentication info for %s to kubectl config.", eksClusterArnString)
	execConfig := api.ExecConfig{
		APIVersion: "client.authentication.k8s.io/v1alpha1",
		Command:    "kubergrunt",
		Args: []string{
			"--loglevel",
			"error",
			"eks",
			"token",
			"--cluster-id",
			eksClusterName,
		},
	}
	authInfo := api.NewAuthInfo()
	authInfo.Exec = &execConfig
	config.AuthInfos[eksClusterArnString] = authInfo
	logger.Infof("Successfully appended authentication info.")
	return nil
}

// AddContextToConfig will add a new context to the kubectl config that ties the provided cluster to the auth info.
func AddContextToConfig(config *api.Config, contextName string, clusterName string, authInfoName string) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Appending context %s to kubectl config.", contextName)
	newContext := api.NewContext()
	newContext.Cluster = clusterName
	newContext.AuthInfo = authInfoName
	config.Contexts[contextName] = newContext
	logger.Infof("Successfully appended context to kubectl config.")
	return nil
}

// LoadConfigFromPath will load a ClientConfig object from a file path that points to a location on disk containing a
// kubectl config.
func LoadConfigFromPath(path string) clientcmd.ClientConfig {
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: path},
		&clientcmd.ConfigOverrides{})
	return config
}

// LoadApiClientConfig will load a ClientConfig object from a file path that points to a location on disk containing a
// kubectl config, with the requested context loaded.
func LoadApiClientConfig(path string, context string) (*restclient.Config, error) {
	overrides := clientcmd.ConfigOverrides{}
	if context != "" {
		overrides.CurrentContext = context
	}
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: path},
		&overrides)
	return config.ClientConfig()
}

// KubeConfigPathFromHomeDir returns a string to the default Kubernetes config path in the home directory. This will
// error if the home directory can not be determined.
func KubeConfigPathFromHomeDir() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(home, ".kube", "config")
	return configPath, err
}

// INITIAL_BLANK_KUBECONFIG is a bare, empty kubeconfig
const INITIAL_BLANK_KUBECONFIG = `apiVersion: v1
clusters: []
contexts: []
current-context: ""
kind: Config
preferences: {}
users: []
`
