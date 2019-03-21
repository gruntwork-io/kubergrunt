package tls

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/kubergrunt/kubectl"
)

// This test will generate a CA certificate key pair, and then use the generated CA certificate key pair to issue a
// signed TLS certificate key pair. Note that this test is designed this way, because it is difficult to test issuing a
// signed TLS certificate key pair without having generated the CA certificate key pair.
func TestGenerateAndStoreAsK8SSecret(t *testing.T) {
	t.Parallel()

	// Create a namespace so we don't collide with other tests
	ttKubectlOptions := k8s.NewKubectlOptions("", "")
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)
	ttKubectlOptions.Namespace = namespace

	// Setup the option/flags for the generator
	kubectlOptions := kubectl.GetTestKubectlOptions(t)
	sampleTlsOptions := SampleTlsOptions(ECDSAAlgorithm)

	// First pass: the CA options
	caSecretName := strings.ToLower(random.UniqueId())
	caSecretOptions := KubernetesSecretOptions{
		Name:        caSecretName,
		Namespace:   namespace,
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	// Generate the CA TLS certificate
	caFilenameBase := random.UniqueId()
	err := GenerateAndStoreAsK8SSecret(
		kubectlOptions,
		caSecretOptions,
		KubernetesSecretOptions{},
		true,
		caFilenameBase,
		sampleTlsOptions,
	)
	require.NoError(t, err)

	// Now setup and generate a signed certificate key pair derived from the CA certificate key pair we just generated.
	secretName := strings.ToLower(random.UniqueId())
	secretOptions := KubernetesSecretOptions{
		Name:        secretName,
		Namespace:   namespace,
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	// Generate the CA TLS certificate
	filenameBase := random.UniqueId()
	err = GenerateAndStoreAsK8SSecret(
		kubectlOptions,
		secretOptions,
		caSecretOptions,
		false,
		filenameBase,
		sampleTlsOptions,
	)
	require.NoError(t, err)

	// Finally validate we can verify the signature
	validateTLSKeyPairInSecret(t, ttKubectlOptions, secretOptions)
}

// This test will test that GenerateAndStoreAsK8SSecret errors if it can't find the CA certificate key pair.
func TestGenerateAndStoreAsK8SSecretErrorsIfCADoesNotExist(t *testing.T) {
	t.Parallel()

	// Create a namespace so we don't collide with other tests
	ttKubectlOptions := k8s.NewKubectlOptions("", "")
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)
	ttKubectlOptions.Namespace = namespace

	// Setup the option/flags for the generator
	kubectlOptions := kubectl.GetTestKubectlOptions(t)
	sampleTlsOptions := SampleTlsOptions(ECDSAAlgorithm)
	secretName := strings.ToLower(random.UniqueId())
	secretOptions := KubernetesSecretOptions{
		Name:        secretName,
		Namespace:   namespace,
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	// Attempt to generate the TLS certificate, but verify it failed in looking up the CA certificate key pair
	filenameBase := random.UniqueId()
	err := GenerateAndStoreAsK8SSecret(
		kubectlOptions,
		secretOptions,
		secretOptions,
		false,
		filenameBase,
		sampleTlsOptions,
	)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), fmt.Sprintf("secrets \"%s\" not found", secretName)))
}

// This test will test that GenerateAndStoreAsK8SSecret supports annotating and labeling the generated Secret.
func TestGenerateAndStoreAsK8SSecretSupportsTagging(t *testing.T) {
	t.Parallel()

	// Create a namespace so we don't collide with other tests
	ttKubectlOptions := k8s.NewKubectlOptions("", "")
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)
	ttKubectlOptions.Namespace = namespace

	// Setup the option/flags for the generator
	kubectlOptions := kubectl.GetTestKubectlOptions(t)
	sampleTlsOptions := SampleTlsOptions(ECDSAAlgorithm)
	secretName := strings.ToLower(random.UniqueId())
	secretOptions := KubernetesSecretOptions{
		Name:      secretName,
		Namespace: namespace,
		Labels: map[string]string{
			"gruntwork.io/test-name": t.Name(),
		},
		Annotations: map[string]string{
			"gruntwork.io/test-name-annotation": t.Name(),
		},
	}

	// Generate the TLS certificate and then verify it created a Kubernetes Secret with the provided label and
	// annotation.
	err := GenerateAndStoreAsK8SSecret(
		kubectlOptions,
		secretOptions,
		KubernetesSecretOptions{},
		true,
		"tls",
		sampleTlsOptions,
	)
	require.NoError(t, err)
	secret := k8s.GetSecret(t, ttKubectlOptions, secretName)
	require.NotNil(t, secret)
	assert.Equal(t, secret.Labels["gruntwork.io/test-name"], t.Name())
	assert.Equal(t, secret.Annotations["gruntwork.io/test-name-annotation"], t.Name())
}

// This test will test that GenerateAndStoreAsK8SSecret annotates the generated secret with additional information that
// helps track it later. Specifically:
// - private key algorithm
// - filename base used
func TestGenerateAndStoreAsK8SSecretAnnotatesAdditionalMetadata(t *testing.T) {
	t.Parallel()

	// Make sure to test both algorithms
	for _, algorithm := range PrivateKeyAlgorithms {
		// Capture range variable, as it might change by the time the test uses it since it is scoped outside this
		// block.
		algorithm := algorithm

		t.Run(algorithm, func(t *testing.T) {
			t.Parallel()

			// Create a namespace so we don't collide with other tests
			ttKubectlOptions := k8s.NewKubectlOptions("", "")
			namespace := strings.ToLower(random.UniqueId())
			k8s.CreateNamespace(t, ttKubectlOptions, namespace)
			defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)
			ttKubectlOptions.Namespace = namespace

			// Setup the option/flags for the generator
			kubectlOptions := kubectl.GetTestKubectlOptions(t)
			sampleTlsOptions := SampleTlsOptions(algorithm)
			secretName := strings.ToLower(random.UniqueId())
			secretOptions := KubernetesSecretOptions{
				Name:        secretName,
				Namespace:   namespace,
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			}

			// Generate the TLS certificate and then verify it created a Kubernetes Secret in the right namespace with the right
			// name.
			filenameBase := random.UniqueId()
			err := GenerateAndStoreAsK8SSecret(
				kubectlOptions,
				secretOptions,
				KubernetesSecretOptions{},
				true,
				filenameBase,
				sampleTlsOptions,
			)
			require.NoError(t, err)
			secret := k8s.GetSecret(t, ttKubectlOptions, secretName)
			require.NotNil(t, secret)

			// Validate the secret has the expected annotations
			assert.Equal(t, secret.Annotations[kubernetesSecretPrivateKeyAlgorithmAnnotationKey], algorithm)
			assert.Equal(t, secret.Annotations[kubernetesSecretFileNameBaseAnnotationKey], filenameBase)
		})
	}
}

func validateTLSKeyPairInSecret(
	t *testing.T,
	kubectlOptions *k8s.KubectlOptions,
	secretOptions KubernetesSecretOptions,
) {
	tlsPath, err := ioutil.TempDir("", "")
	require.NoError(t, err)

	secret := k8s.GetSecret(t, kubectlOptions, secretOptions.Name)
	filenameBase := secret.Annotations[kubernetesSecretFileNameBaseAnnotationKey]
	caCertPath := filepath.Join(tlsPath, "ca.crt")
	require.NoError(t, ioutil.WriteFile(caCertPath, secret.Data["ca.crt"], 0600))
	certPath := filepath.Join(tlsPath, "tls.crt")
	require.NoError(t, ioutil.WriteFile(certPath, secret.Data[fmt.Sprintf("%s.crt", filenameBase)], 0600))

	// Verify the signed certificate is indeed signed by the CA certificate
	verifyCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"verify", "-CAfile", caCertPath, certPath},
	}
	shell.RunCommand(t, verifyCmd)
}
