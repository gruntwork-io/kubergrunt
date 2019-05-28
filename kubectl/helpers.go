package kubectl

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LabelsToListOptions takes a map of label keys/values and returns ListOptions with LabelSelector
func LabelsToListOptions(labels map[string]string) metav1.ListOptions {
	var selectors []string
	for k, v := range labels {
		selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
	}
	return metav1.ListOptions{
		LabelSelector: strings.Join(selectors, ","),
	}
}
