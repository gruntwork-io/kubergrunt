package helm

import (
	"fmt"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/helm/cmd/helm/installer"
	"k8s.io/helm/pkg/helm/portforwarder"
	helmkube "k8s.io/helm/pkg/kube"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
	"github.com/gruntwork-io/kubergrunt/tls"
)

// DefaultTillerConnectionTimeout is the number of seconds to wait before timing out the connection to Tiller
const DefaultTillerConnectionTimeout = 300

const TillerDeploymentName = "tiller-deploy"

// InstallTiller will install Tiller onto the Kubernetes cluster.
// Returns the Tiller image being installed.
func InstallTiller(
	kubectlOptions *kubectl.KubectlOptions,
	caKeyPairPath tls.CertificateKeyPairPath,
	tillerKeyPairPath tls.CertificateKeyPairPath,
	tillerNamespace string,
	serviceAccountName string,
) (string, error) {
	client, err := kubectl.GetKubernetesClientFromFile(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	if err != nil {
		return "", err
	}

	options := installer.Options{}

	// RBAC options
	options.Namespace = tillerNamespace
	options.ServiceAccount = serviceAccountName
	options.AutoMountServiceAccountToken = true

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
		return "", errors.WithStackTrace(err)
	}
	return options.SelectImage(), nil
}

// WaitForTiller will poll Kubernetes until Tiller is available, and then verify the Tiller instance is up.
// This is ported from the helm client: https://github.com/helm/helm/blob/master/cmd/helm/init.go#L322
func WaitForTiller(
	kubectlOptions *kubectl.KubectlOptions,
	helmHome string,
	newImage string,
	tillerNamespace string,
) error {
	logger := logging.GetProjectLogger()

	kubeClient, err := kubectl.GetKubernetesClientFromFile(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	if err != nil {
		return err
	}

	sleepTime := 500 * time.Millisecond
	deadlinePollingChan := time.NewTimer(time.Duration(DefaultTillerConnectionTimeout) * time.Second).C
	checkTillerPodTicker := time.NewTicker(sleepTime)
	doneChan := make(chan bool)

	defer checkTillerPodTicker.Stop()

	go func() {
		logger.Infof("Initiating polling of Tiller pod")
		for range checkTillerPodTicker.C {
			// Wait for the deployment to scale up
			deployment, err := kubeClient.Extensions().Deployments(tillerNamespace).Get(TillerDeploymentName, metav1.GetOptions{})
			if err != nil {
				logger.Warnf("Tiller deployment doesn't exist yet: %s", err)
				logger.Warnf("Trying again in %s", sleepTime)
				continue
			}
			if deployment.Status.AvailableReplicas == 0 {
				logger.Infof("Tiller is not available yet. Sleeping for %s.", sleepTime)
				continue
			}

			// Now query the pods
			filters := metav1.ListOptions{LabelSelector: ""}
			pods, err := kubectl.ListPods(kubectlOptions, tillerNamespace, filters)
			if err != nil {
				logger.Warnf("Error trying to lookup pods: %s", err)
				logger.Warnf("Trying again in %s", sleepTime)
				continue
			}
			if len(pods) == 0 {
				logger.Infof("No Tiller pods found yet. Sleeping for %s.", sleepTime)
				continue
			}
			readyPods := 0
			for _, pod := range pods {
				if kubectl.IsPodReady(pod) {
					readyPods++
				}
			}
			if readyPods == 0 {
				logger.Infof("No Tiller pods ready yet. Sleeping for %s.", sleepTime)
				continue
			}

			// Wait for the image to be replaced with the expected one
			image, err := portforwarder.GetTillerPodImage(kubeClient.CoreV1(), tillerNamespace)
			if err != nil {
				logger.Warnf("Could not create port forward to Tiller pod to query image: %s", err)
				logger.Warnf("Trying again in %s", sleepTime)
				continue
			}

			if image != newImage {
				logger.Infof("Tiller is not running expected image yet. Running (%s), Expected (%s)", image, newImage)
				continue
			}

			logger.Infof("Detected Tiller is ready. Ending polling.")
			doneChan <- true
			break
		}
	}()

	// Poll until deadline
	for {
		select {
		case <-deadlinePollingChan:
			return errors.WithStackTrace(TillerDeployWaitTimeoutError{Namespace: tillerNamespace})
		case <-doneChan:
			return nil
		}
	}
}

// VerifyTiller pings the Tiller host with the helm client configured using the settings in the provided helmHome to
// verify it is up.
func VerifyTiller(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	helmHome string,
) error {
	logger := logging.GetProjectLogger()

	kubeClient, err := kubectl.GetKubernetesClientFromFile(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	if err != nil {
		return err
	}

	logger.Infof("Setting up connection to Tiller Pod in Namespace %s", tillerNamespace)
	tillerTunnel, err := SetupConnection(kubeClient, kubectlOptions, tillerNamespace)
	if err != nil {
		logger.Errorf("Error trying to open connection to Tiller Pod in Namespace %s: %s", tillerNamespace, err)
		return err
	}
	defer tillerTunnel.Close()
	tillerHost := fmt.Sprintf("127.0.0.1:%d", tillerTunnel.Local)
	logger.Infof("Successfully opened tunnel to Tiller Pod in Namespace %s: %s", tillerNamespace, tillerHost)

	logger.Infof("Setting up new helm client with home %s and pinging Tiller", helmHome)
	helmClient, err := NewHelmClient(
		tillerHost,
		DefaultTillerConnectionTimeout,
		helmHome,
	)
	if err != nil {
		logger.Errorf("Error setting up helm client: %s", err)
		return err
	}
	if err := helmClient.PingTiller(); err != nil {
		logger.Errorf("Error pinging Tiller: %s", err)
		return errors.WithStackTrace(TillerPingError{Namespace: tillerNamespace, UnderlyingError: err})
	}
	logger.Infof("Successfully pinged Tiller")

	return nil
}

// SetupConnection will setup a tunnel to a deployed Tiller instance.
func SetupConnection(kubeClient *kubernetes.Clientset, kubectlOptions *kubectl.KubectlOptions, tillerNamespace string) (*helmkube.Tunnel, error) {
	config, err := kubectl.LoadApiClientConfig(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	if err != nil {
		return nil, err
	}

	tillerTunnel, err := portforwarder.New(tillerNamespace, kubeClient, config)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return tillerTunnel, nil
}
