package helm

import (
	"crypto/x509/pkix"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/tls"
)

func TestGenerateCertificateKeyPairs(t *testing.T) {
	t.Parallel()

	for _, algorithm := range tls.PrivateKeyAlgorithms {
		// Capture range variable so that it doesn't change for code in this block
		algorithm := algorithm
		t.Run(algorithm, func(t *testing.T) {
			t.Parallel()
			validateGenerateCertificateKeyPair(t, algorithm)
		})
	}
}

func TestValidateRequiredResourcesForDeploy(t *testing.T) {
	t.Parallel()
	kubectlOptions := getTestKubectlOptions(t)

	// No Namespace or ServiceAccount
	err := validateRequiredResourcesForDeploy(kubectlOptions, "this-namespace-doesnt-exist", "this-service-account-doesnt-exist")
	assert.Error(t, err)

	// No ServiceAccount
	err = validateRequiredResourcesForDeploy(kubectlOptions, "default", "this-service-account-doesnt-exist")
	assert.Error(t, err)

	// No Namespace
	err = validateRequiredResourcesForDeploy(kubectlOptions, "this-namespace-doesnt-exist", "default")
	assert.Error(t, err)

	// Both Namespace and ServiceAccount exist
	err = validateRequiredResourcesForDeploy(kubectlOptions, "default", "default")
	assert.NoError(t, err)
}

// Test that we can:
// 1. Generate certificate key pairs for use with Tiller
// 2. Upload certificate key pairs to Kubernetes secrets
// 3. Deploy Helm with TLS enabled in the specified namespace
func TestDeployWorkflow(t *testing.T) {
	t.Parallel()
	kubectlOptions := getTestKubectlOptions(t)
	terratestKubectlOptions := k8s.NewKubectlOptions("", "")
	tlsOptions := sampleTlsOptions(tls.ECDSAAlgorithm)
	namespaceName := strings.ToLower(random.UniqueId())
	serviceAccountName := fmt.Sprintf("%s-service-account", namespaceName)

	k8s.CreateNamespace(t, terratestKubectlOptions, namespaceName)
	defer func() {
		terratestKubectlOptions.Namespace = ""
		k8s.DeleteNamespace(t, terratestKubectlOptions, namespaceName)
	}()
	terratestKubectlOptions.Namespace = namespaceName

	k8s.CreateServiceAccount(t, terratestKubectlOptions, serviceAccountName)

	err := Deploy(kubectlOptions, namespaceName, serviceAccountName, tlsOptions)
	assert.NoError(t, err)

	// Check tiller pod is in chosen namespace
	tillerPodName := validateTillerPodDeployedInNamespace(t, terratestKubectlOptions)

	// Check tiller pod is launched with the right service account
	validateTillerPodServiceAccount(t, terratestKubectlOptions, tillerPodName, serviceAccountName)

	// Check tiller pod uses secrets instead of configmap as metadata backend
	validateTillerPodUsesSecrets(t, terratestKubectlOptions)

	// Check tiller pod TLS
	validateTillerPodUsesTLS(t, terratestKubectlOptions)
}

// validateTillerPodDeployedInNamespace validates that the tiller pod was deployed into the provided namespace and
// returns the name of the pod.
func validateTillerPodDeployedInNamespace(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions) string {
	tillerPodName, err := k8s.RunKubectlAndGetOutputE(
		t,
		terratestKubectlOptions,
		"get",
		"pods",
		"-o",
		"name",
		"-l",
		"app=helm,name=tiller",
	)
	assert.NoError(t, err)
	assert.NotEqual(t, tillerPodName, "")
	return strings.TrimLeft(tillerPodName, "pod/")
}

// validateTillerPodDeployedInNamespace validates that the tiller pod was deployed with the provided service account
func validateTillerPodServiceAccount(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions, tillerPodName string, serviceAccountName string) {
	pod := k8s.GetPod(t, terratestKubectlOptions, tillerPodName)
	assert.Equal(t, pod.Spec.ServiceAccountName, serviceAccountName)
}

// validateTillerPodUsesSecrets validates that the tiller pod is deployed with using secrets for metadata instead of
// configmaps.
func validateTillerPodUsesSecrets(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions) {
	// First make sure there are no configmaps in the namespace
	maybeConfigMap, err := k8s.RunKubectlAndGetOutputE(
		t,
		terratestKubectlOptions,
		"get",
		"configmaps",
		"-o",
		"name",
	)
	assert.NoError(t, err)
	assert.Equal(t, maybeConfigMap, "")
	// Then make sure that there is a secret named `tiller-secret` in the namespace
	maybeSecret, err := k8s.RunKubectlAndGetOutputE(
		t,
		terratestKubectlOptions,
		"get",
		"secrets",
		"-o",
		"name",
		"tiller-secret",
	)
	assert.NoError(t, err)
	assert.NotEqual(t, maybeSecret, "")
}

// validateTillerPodUsesTLS verifies that the deployed tiller pod has TLS certs configured.
func validateTillerPodUsesTLS(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions) {
	secret := k8s.GetSecret(t, terratestKubectlOptions, "tiller-secret")
	for _, expectedKey := range []string{"tls.key", "ca.crt", "tls.crt"} {
		_, hasKey := secret.Data[expectedKey]
		assert.True(t, hasKey)
	}
}

func validateGenerateCertificateKeyPair(t *testing.T, algorithm string) {
	tmpDir, err := ioutil.TempDir("", algorithm)
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tlsOptions := sampleTlsOptions(algorithm)
	caCertificateKeyPair, signedCertificateKeyPair, err := generateCertificateKeyPairs(
		tlsOptions,
		algorithm,
		tmpDir,
	)

	// Make sure the keys are compatible with cert
	validateKeyCompatibility(t, caCertificateKeyPair)
	validateKeyCompatibility(t, signedCertificateKeyPair)

	// Make sure the signed cert is actually signed by the CA
	validateSignedCert(t, caCertificateKeyPair.CertificatePath, signedCertificateKeyPair.CertificatePath)
}

// validateSignedCert makes sure the cert was signed by the CA cert
func validateSignedCert(t *testing.T, caCertPath string, signedCertPath string) {
	verifyCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"verify", "-CAfile", caCertPath, signedCertPath},
	}
	shell.RunCommand(t, verifyCmd)
}

// validateKeyCompatibility makes sure the keys and certs match
func validateKeyCompatibility(t *testing.T, certKeyPair tls.CertificateKeyPairPath) {
	// Verify the certificate are for the key pair. This can be done by validating that the extracted public keys are
	// all the same. This check does not depend on the key pair algorithm, but is less robust than algorithm dependent
	// checks (e.g checking the modulus for RSA).
	certPubKeyCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"x509", "-inform", "PEM", "-in", certKeyPair.CertificatePath, "-pubkey", "-noout"},
	}
	certPubKey := shell.RunCommandAndGetOutput(t, certPubKeyCmd)
	keyPubFromPrivCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"pkey", "-pubout", "-inform", "PEM", "-in", certKeyPair.PrivateKeyPath, "-outform", "PEM"},
	}
	keyPubFromPriv := shell.RunCommandAndGetOutput(t, keyPubFromPrivCmd)
	pubKey, err := ioutil.ReadFile(certKeyPair.PublicKeyPath)
	assert.NoError(t, err)

	assert.Equal(t, strings.TrimSpace(certPubKey), strings.TrimSpace(string(pubKey)))
	assert.Equal(t, strings.TrimSpace(certPubKey), strings.TrimSpace(keyPubFromPriv))

}

func sampleTlsOptions(algorithm string) tls.TLSOptions {
	options := tls.TLSOptions{
		DistinguishedName: pkix.Name{
			CommonName:         "gruntwork.io",
			Organization:       []string{"Gruntwork"},
			OrganizationalUnit: []string{"IT"},
			Locality:           []string{"Phoenix"},
			Province:           []string{"AZ"},
			Country:            []string{"US"},
		},
		ValidityTimeSpan:    1 * time.Hour,
		PrivateKeyAlgorithm: algorithm,
		RSABits:             2048,
		ECDSACurve:          tls.P256Curve,
	}
	return options
}

func getTestKubectlOptions(t *testing.T) *kubectl.KubectlOptions {
	kubeConfigPath, err := k8s.GetKubeConfigPathE(t)
	require.NoError(t, err)
	return kubectl.NewKubectlOptions("", kubeConfigPath)
}
