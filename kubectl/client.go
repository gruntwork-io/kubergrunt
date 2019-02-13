package kubectl

import (
	"k8s.io/client-go/kubernetes"

	// The following line loads the gcp plugin which is required to authenticate against GKE clusters.
	// See: https://github.com/kubernetes/client-go/issues/242
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/gruntwork-io/kubergrunt/logging"
)

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
