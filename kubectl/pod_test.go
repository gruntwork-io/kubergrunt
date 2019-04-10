package kubectl

import (
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestListPodsReturnsPods(t *testing.T) {
	t.Parallel()

	kubeConfigPath, err := k8s.GetKubeConfigPathE(t)
	require.NoError(t, err)
	kubectlOptions := &KubectlOptions{ConfigPath: kubeConfigPath}

	// There are always Pods in the kube-system namespace in any kubernetes cluster
	pods, err := ListPods(kubectlOptions, "kube-system", metav1.ListOptions{})
	require.NoError(t, err)
	require.True(t, len(pods) > 0)
}
