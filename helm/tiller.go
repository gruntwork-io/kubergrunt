package helm

import (
	"fmt"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/helm/cmd/helm/installer"
	"k8s.io/helm/pkg/helm/portforwarder"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/tls"
)

// DefaultTillerConnectionTimeout is the number of seconds to wait before timing out the connection to Tiller
const DefaultTillerConnectionTimeout = 300

// installTiller will install Tiller onto the Kubernetes cluster.
func installTiller(
	kubectlOptions *kubectl.KubectlOptions,
	caKeyPairPath tls.CertificateKeyPairPath,
	tillerKeyPairPath tls.CertificateKeyPairPath,
	tillerNamespace string,
	serviceAccountName string,
) error {
	client, err := kubectl.GetKubernetesClientFromFile(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	if err != nil {
		return err
	}

	options := installer.Options{}

	// RBAC options
	options.Namespace = tillerNamespace
	options.ServiceAccount = serviceAccountName

	// TLS options
	options.EnableTLS = true
	options.VerifyTLS = true
	options.TLSKeyFile = tillerKeyPairPath.PrivateKeyPath
	options.TLSCertFile = tillerKeyPairPath.CertificatePath
	options.TLSCaCertFile = caKeyPairPath.CertificatePath

	// Use Secrets instead of ConfigMap to track metadata
	options.Values = []string{
		"spec.template.spec.containers[0].command={/tiller,--storage=secret}",
	}

	// Actually perform the deployment
	err = installer.Install(client, &options)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// Now wait for Tiller to come up
	err = waitForTiller(client, kubectlOptions, options.SelectImage(), tillerNamespace)
	if err != nil {
		return err
	}
	// establish a connection to Tiller now that we've effectively guaranteed it's available
	tillerHost, err := setupConnection(client, kubectlOptions)
	if err != nil {
		return err
	}
	helmClient, err := NewHelmClient(
		tillerHost,
		DefaultTillerConnectionTimeout,
		caKeyPairPath.CertificatePath,
		tillerKeyPairPath,
	)
	if err != nil {
		return err
	}
	if err := helmClient.PingTiller(); err != nil {
		// TODO: return a typed error
		return errors.WithStackTrace(fmt.Errorf("could not ping Tiller: %s", err))
	}
	return nil

}

// waitForTiller will poll Kubernetes until Tiller is available, and then verify the Tiller instance is up.
// This is ported from the helm client: https://github.com/helm/helm/blob/master/cmd/helm/init.go#L322
func waitForTiller(
	kubeClient *kubernetes.Clientset,
	kubectlOptions *kubectl.KubectlOptions,
	image string,
	tillerNamespace string,
) error {
	// TODO: Add more logging in general

	deadlinePollingChan := time.NewTimer(time.Duration(DefaultTillerConnectionTimeout) * time.Second).C
	checkTillerPodTicker := time.NewTicker(500 * time.Millisecond)
	doneChan := make(chan bool)

	defer checkTillerPodTicker.Stop()

	go func() {
		for range checkTillerPodTicker.C {
			image, err := portforwarder.GetTillerPodImage(kubeClient.CoreV1(), tillerNamespace)
			if err == nil && image == newImage {
				doneChan <- true
				break
			}
		}
	}()

	for {
		select {
		case <-deadlinePollingChan:
			// TODO: replace with a typed error
			return errors.WithStaclTrace(fmt.Errorf("tiller was not found. polling deadline exceeded"))
		case <-doneChan:
			return nil
		}
	}
}

// setupConnection will setup a tunnel to a deployed Tiller instance.
func setupConnection(kubeClient *kubernetes.Clientset, kubectlOptions *kubectl.KubectlOptions) (string, error) {
	config, err := kubectl.LoadApiClientConfig(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	if err != nil {
		return "", err
	}

	tillerTunnel, err = portforwarder.New(settings.TillerNamespace, client, config)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	tillerHost = fmt.Sprintf("127.0.0.1:%d", tillerTunnel.Local)
	return tillerHost, nil
}
