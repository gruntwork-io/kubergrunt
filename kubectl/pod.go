package kubectl

import (
	"context"

	"github.com/gruntwork-io/go-commons/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListPods will look for pods in the given namespace and return them.
func ListPods(options *KubectlOptions, namespace string, filters metav1.ListOptions) ([]corev1.Pod, error) {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return nil, err
	}

	resp, err := client.CoreV1().Pods(namespace).List(context.Background(), filters)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return resp.Items, nil
}

// IsPodReady returns True when a Pod is in the Ready status.
func IsPodReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
