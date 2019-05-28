package kubectl

import (
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
)

func GetTestKubectlOptions(t *testing.T) *KubectlOptions {
	kubeConfigPath, err := k8s.GetKubeConfigPathE(t)
	require.NoError(t, err)
	return &KubectlOptions{ConfigPath: kubeConfigPath}
}

func GetKubectlOptions(t *testing.T) (*k8s.KubectlOptions, *KubectlOptions) {
	ttKubectlOptions := k8s.NewKubectlOptions("", "")
	configPath, err := k8s.KubeConfigPathFromHomeDirE()
	require.NoError(t, err)
	kubectlOptions := &KubectlOptions{ConfigPath: configPath}
	return ttKubectlOptions, kubectlOptions
}
