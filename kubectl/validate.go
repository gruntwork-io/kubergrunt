package kubectl

import (
	"context"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ValidateNamespaceExists will return an error if the provided namespace does not exist on the Kubernetes cluster.
func ValidateNamespaceExists(kubectlOptions *KubectlOptions, namespace string) error {
	client, err := GetKubernetesClientFromOptions(kubectlOptions)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	_, err = client.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// ValidateServiceAccountExists will return an error if the provided service account does not exist on the provided
// namespace in the Kubernetes cluster.
func ValidateServiceAccountExists(kubectlOptions *KubectlOptions, namespace string, serviceAccount string) error {
	client, err := GetKubernetesClientFromOptions(kubectlOptions)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	_, err = client.CoreV1().ServiceAccounts(namespace).Get(context.Background(), serviceAccount, metav1.GetOptions{})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}
