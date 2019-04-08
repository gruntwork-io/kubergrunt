package helm

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/logger"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/gruntwork-io/kubergrunt/kubectl"
)

func TestSetKubectlNamespaceSetsTheDefaultNamespace(t *testing.T) {
	t.Parallel()

	// Create a temporary config so that updating it won't affect other tests.
	tmpConfigPath := copyKubeconfigToTempFile(t)
	logger.Logf(t, "Created temp kubeconfig %s", tmpConfigPath)
	defer os.Remove(tmpConfigPath)
	options := &kubectl.KubectlOptions{ConfigPath: tmpConfigPath}
	tempTerratestKubectlOptions := k8s.NewKubectlOptions("", tmpConfigPath)

	// Create a new namespace and create a configmap resource
	namespaceName := strings.ToLower(random.UniqueId())
	terratestKubectlOptions := k8s.NewKubectlOptions("", "")
	defer k8s.DeleteNamespace(t, terratestKubectlOptions, namespaceName)
	k8s.CreateNamespace(t, terratestKubectlOptions, namespaceName)
	terratestKubectlOptions.Namespace = namespaceName
	configMapData := random.UniqueId()
	createTestConfigMap(t, terratestKubectlOptions, configMapData)

	// Now update the default namespace and verify we can get test config map without any namespace moniker
	require.NoError(t, setKubectlNamespaceForCurrentContext(options, namespaceName))
	// We purposefully use the kubectl command line here to verify the default namespace is set
	out, err := k8s.RunKubectlAndGetOutputE(t, tempTerratestKubectlOptions, "get", "configmap", "test-data", "-o", "json")
	require.NoError(t, err)
	configMap := corev1.ConfigMap{}
	require.NoError(t, json.Unmarshal([]byte(out), &configMap))
	require.Equal(t, configMap.Data["data"], configMapData)
}

func createTestConfigMap(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions, data string) {
	clientset, err := k8s.GetKubernetesClientFromOptionsE(t, terratestKubectlOptions)
	require.NoError(t, err)
	configMap := &corev1.ConfigMap{Data: map[string]string{"data": data}}
	configMap.Name = "test-data"
	_, err = clientset.CoreV1().ConfigMaps(terratestKubectlOptions.Namespace).Create(configMap)
	require.NoError(t, err)
}
