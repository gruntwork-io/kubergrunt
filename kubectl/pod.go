package kubectl

import (
	"github.com/gruntwork-io/gruntwork-cli/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListPods will look for pods in the given namespace and return them.
func ListPods(options *KubectlOptions, namespace string, filters metav1.ListOptions) ([]corev1.Pod, error) {
	client, err := GetKubernetesClientFromFile(options.ConfigPath, options.ContextName)
	if err != nil {
		return nil, err
	}

	resp, err := client.CoreV1().Pods(namespace).List(filters)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return resp.Items, nil
}
