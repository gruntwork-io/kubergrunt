package helm

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/tls"
)

func TestStoreCertificateKeyPairAsKubernetesSecretStoresAllFiles(t *testing.T) {
	t.Parallel()

	// Construct kubectl options
	ttKubectlOptions := k8s.NewKubectlOptions("", "")
	configPath, err := k8s.KubeConfigPathFromHomeDirE()
	require.NoError(t, err)
	kubectlOptions := kubectl.NewKubectlOptions("", configPath)

	// Create a namespace so we don't collide with other tests
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	// Now store certificate key pair using the tested function
	baseName := random.UniqueId()
	certificateKeyPairPath := createSampleCertificateKeyPairPath(t)
	err = StoreCertificateKeyPairAsKubernetesSecret(
		kubectlOptions,
		"random-certs",
		namespace,
		map[string]string{},
		map[string]string{},
		baseName,
		certificateKeyPairPath,
		"",
	)
	require.NoError(t, err)

	// Verify the created cert
	ttKubectlOptions.Namespace = namespace
	secret := k8s.GetSecret(t, ttKubectlOptions, "random-certs")
	assert.Equal(t, secret.Data[fmt.Sprintf("%s.crt", baseName)], mustReadFile(t, certificateKeyPairPath.CertificatePath))
	assert.Equal(t, secret.Data[fmt.Sprintf("%s.pem", baseName)], mustReadFile(t, certificateKeyPairPath.PrivateKeyPath))
	assert.Equal(t, secret.Data[fmt.Sprintf("%s.pub", baseName)], mustReadFile(t, certificateKeyPairPath.PublicKeyPath))
	// Verify the ca cert entry doesn't exist, because we set it to empty string
	_, found := secret.Data["ca.crt"]
	assert.False(t, found)
}

func TestStoreCertificateKeyPairAsKubernetesSecretStoresCACert(t *testing.T) {
	t.Parallel()

	// Construct kubectl options
	ttKubectlOptions := k8s.NewKubectlOptions("", "")
	configPath, err := k8s.KubeConfigPathFromHomeDirE()
	require.NoError(t, err)
	kubectlOptions := kubectl.NewKubectlOptions("", configPath)

	// Create a namespace so we don't collide with other tests
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	// Now store certificate key pair using the tested function, with the CA cert
	baseName := random.UniqueId()
	caCertPath := mustAbs(t, "./testfixtures/ca.cert")
	certificateKeyPairPath := createSampleCertificateKeyPairPath(t)
	err = StoreCertificateKeyPairAsKubernetesSecret(
		kubectlOptions,
		"random-certs",
		namespace,
		map[string]string{},
		map[string]string{},
		baseName,
		certificateKeyPairPath,
		caCertPath,
	)
	require.NoError(t, err)

	// Verify the created cert
	ttKubectlOptions.Namespace = namespace
	secret := k8s.GetSecret(t, ttKubectlOptions, "random-certs")
	assert.Equal(t, secret.Data[fmt.Sprintf("%s.crt", baseName)], mustReadFile(t, certificateKeyPairPath.CertificatePath))
	assert.Equal(t, secret.Data[fmt.Sprintf("%s.pem", baseName)], mustReadFile(t, certificateKeyPairPath.PrivateKeyPath))
	assert.Equal(t, secret.Data[fmt.Sprintf("%s.pub", baseName)], mustReadFile(t, certificateKeyPairPath.PublicKeyPath))
	assert.Equal(t, secret.Data["ca.crt"], mustReadFile(t, caCertPath))
}

func mustReadFile(t *testing.T, path string) []byte {
	data, err := ioutil.ReadFile(path)
	require.NoError(t, err)
	return data
}

func createSampleCertificateKeyPairPath(t *testing.T) tls.CertificateKeyPairPath {
	// Load in pregenerated test certificate key pairs. These are generated using openssl and are not used anywhere.
	// To regenerate them, run:
	//   openssl genrsa -out ./ca.key.pem 4096
	//   openssl req -key ca.key.pem -new -x509 -days 7300 -sha256 -out ca.cert
	//   openssl rsa -in ca.key.pem -pubout > ca.pub
	//   openssl genrsa -out ./tiller.key.pem 4096
	//   openssl req -key tiller.key.pem -new -sha256 -out tiller.csr.pem
	//   openssl x509 -req -CA ca.cert -CAkey ca.key.pem -CAcreateserial -in tiller.csr.pem -out tiller.cert -days 365
	//   openssl rsa -in tiller.key.pem -pubout > tiller.pub
	return tls.CertificateKeyPairPath{
		CertificatePath: mustAbs(t, "./testfixtures/tiller.cert"),
		PrivateKeyPath:  mustAbs(t, "./testfixtures/tiller.key.pem"),
		PublicKeyPath:   mustAbs(t, "./testfixtures/tiller.pub"),
	}
}

func mustAbs(t *testing.T, path string) string {
	absPath, err := filepath.Abs(path)
	require.NoError(t, err)
	return absPath
}
