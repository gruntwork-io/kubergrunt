package kubectl

import (
	"time"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gruntwork-io/kubergrunt/logging"
)

// GetIngress returns a Kubernetes Ingress resource in the provided namespace with the given name.
func GetIngress(options *KubectlOptions, namespace string, ingressName string) (*extensionsv1beta1.Ingress, error) {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return nil, err
	}

	return client.ExtensionsV1beta1().Ingresses(namespace).Get(ingressName, metav1.GetOptions{})
}

// IsIngressAvailable returns true if the Ingress endpoint is provisioned and available.
func IsIngressAvailable(ingress *extensionsv1beta1.Ingress) bool {
	// Ingress is ready if it has at least one endpoint
	endpoints := ingress.Status.LoadBalancer.Ingress
	return len(endpoints) > 0
}

// GetIngressEndpoints returns all the available ingress endpoints (preferring hostnames, and if unavailable, returning
// IPs). Note that if no endpoints are available, returns empty list.
func GetIngressEndpoints(ingress *extensionsv1beta1.Ingress) []string {
	endpointStatuses := ingress.Status.LoadBalancer.Ingress
	endpoints := []string{}
	for _, endpointStatus := range endpointStatuses {
		endpoint := endpointStatus.Hostname
		if endpoint == "" {
			endpoint = endpointStatus.IP
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

// WaitUntilIngressEndpointProvisioned continuously checks the Ingress resource until the endpoint is provisioned or if
// it times out.
func WaitUntilIngressEndpointProvisioned(
	options *KubectlOptions,
	namespace string,
	ingressName string,
	maxRetries int,
	sleepBetweenRetries time.Duration,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Waiting for Ingress %s (Namespace: %s) endpoint to be provisioned.", ingressName, namespace)

	for i := 0; i < maxRetries; i++ {
		logger.Info("Retrieving Ingress and checking if the endpoint is provisioned.")

		ingress, err := GetIngress(options, namespace, ingressName)
		if err == nil && IsIngressAvailable(ingress) {
			endpoints := GetIngressEndpoints(ingress)
			logger.Infof("Endpoint for Ingress %s (Namespace: %s): %v", ingressName, namespace, endpoints)
			return nil
		}

		logger.Warnf("Endpoint for Ingress %s (Namespace: %s) is not provisioned yet", ingressName, namespace)
		logger.Infof("Waiting for %s...", sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}
	return errors.WithStackTrace(ProvisionIngressEndpointTimeoutError{ingressName: ingressName, namespace: namespace})
}
