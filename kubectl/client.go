package kubectl

import (
	"encoding/base64"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	// The following line loads the gcp plugin which is required to authenticate against GKE clusters.
	// See: https://github.com/kubernetes/client-go/issues/242
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/gruntwork-io/kubergrunt/logging"
)

// GetKubernetesClientFromOptions returns a Kubernetes API client given a KubectlOptions object. Constructs the client
// based on the information in the struct:
// - If Server is set, assume direct auth methods and use Server, Base64PEMCertificateAuthority, and BearerToken to
//   construct authenticated client.
// - Else, use ConfigPath and ContextName to load the config from disk and setup the client to use the auth method
//   provided in the context.
func GetKubernetesClientFromOptions(kubectlOptions *KubectlOptions) (*kubernetes.Clientset, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Loading Kubernetes Client")

	if kubectlOptions.Server == "" {
		logger.Infof("No direct auth methods provided. Using config and context.")
		return GetKubernetesClientFromFile(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	} else {
		logger.Infof("Using direct auth methods to setup client.")
		return GetKubernetesClientFromAuthInfo(
			kubectlOptions.Server,
			kubectlOptions.Base64PEMCertificateAuthority,
			kubectlOptions.BearerToken,
		)
	}
}

// GetKubernetesClientFromFile returns a Kubernetes API client given the kubernetes config file path.
func GetKubernetesClientFromFile(kubeConfigPath string, contextName string) (*kubernetes.Clientset, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Loading Kubernetes Client with config %s and context %s", kubeConfigPath, contextName)

	// Load API config (instead of more low level ClientConfig)
	config, err := LoadApiClientConfig(kubeConfigPath, contextName)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

// GetKubernetesClientFromAuthInfo returns a Kubernetes API client given authentication info provided directly through
// function parameters as opposed to a config.
func GetKubernetesClientFromAuthInfo(server string, certificateAuthorityData string, token string) (*kubernetes.Clientset, error) {
	caData, err := base64.StdEncoding.DecodeString(certificateAuthorityData)
	if err != nil {
		return nil, err
	}

	// TODO: figure out where CA data fits?
	config := &rest.Config{
		Host:        server,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: false,
			CAData:   caData,
		},
	}
	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}
	clientset := kubernetes.New(restClient)
	return clientset, nil
}
