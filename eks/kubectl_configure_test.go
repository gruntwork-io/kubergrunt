package eks

import (
	"encoding/base64"
	"io/ioutil"
	"net/url"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/kubergrunt/kubectl"
)

func TestEksKubectlConfigureHonorsKubeConfigPath(t *testing.T) {
	t.Parallel()

	kubeconfigPath := generateTempConfig(t)
	defer os.Remove(kubeconfigPath)

	originalKubeconfig := k8s.LoadConfigFromPath(kubeconfigPath)
	originalRawConfig, err := originalKubeconfig.RawConfig()
	require.NoError(t, err)

	uniqueID := random.UniqueId()
	anotherUniqueID := random.UniqueId()
	b64CertificateAuthorityData := base64.StdEncoding.EncodeToString([]byte(anotherUniqueID))
	mockCluster := &eks.Cluster{
		Arn:                  aws.String("arn:aws:eks:us-east-2:111111111111:cluster/" + uniqueID),
		Name:                 aws.String(uniqueID),
		Endpoint:             aws.String("gruntwork.io"),
		CertificateAuthority: &eks.Certificate{Data: aws.String(b64CertificateAuthorityData)},
	}
	options := kubectl.NewKubectlOptions(t.Name(), kubeconfigPath)
	err = ConfigureKubectlForEks(mockCluster, options)
	require.NoError(t, err)

	// Verify config was updated
	kubeconfig := k8s.LoadConfigFromPath(kubeconfigPath)
	rawConfig, err := kubeconfig.RawConfig()
	require.NoError(t, err)
	require.NotEqual(t, rawConfig, originalRawConfig)
}

func generateTempConfig(t *testing.T) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()

	_, err = tmpfile.WriteString(BASIC_CONFIG)
	require.NoError(t, err)
	return tmpfile.Name()
}

// Various example configs used in testing the config manipulation functions

const BASIC_CONFIG = `apiVersion: v1
clusters:
- cluster:
    certificate-authority: /home/terratest/.minikube/ca.crt
    server: https://172.17.0.48:8443
  name: minikube
contexts:
- context:
    cluster: minikube
    user: minikube
  name: minikube
current-context: minikube
kind: Config
preferences: {}
users:
- name: minikube
  user:
    client-certificate: /home/terratest/.minikube/client.crt
    client-key: /home/terratest/.minikube/client.key
`
