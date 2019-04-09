package kubectl

import (
	"k8s.io/client-go/kubernetes"

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

	config, err := LoadApiClientConfigFromOptions(kubectlOptions)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}
