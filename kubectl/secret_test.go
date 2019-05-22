package kubectl

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test that we can successfully add a secret value from a file to a secret struct prepared by PrepareSecret.
func TestAddSecretFromFileHappyPath(t *testing.T) {
	t.Parallel()

	tmpfile, contents := createRandomFile(t)
	secret := PrepareSecret(
		"test-namespace",
		"test-name",
		map[string]string{},
		map[string]string{},
	)
	require.NoError(t, AddToSecretFromFile(secret, "data", tmpfile))
	assert.Equal(t, string(secret.Data["data"]), contents)
}

// Test that AddToSecretFromFile will error out if the file doesn't exist (and not add anything to the secret)
func TestAddSecretFromFileWhenFileDoesNotExist(t *testing.T) {
	t.Parallel()

	secret := PrepareSecret(
		"test-namespace",
		"test-name",
		map[string]string{},
		map[string]string{},
	)
	require.Error(t, AddToSecretFromFile(secret, "data", "this-file-should-not-exist.go"))
	_, exists := secret.Data["data"]
	assert.False(t, exists)
}

// Test that AddToSecretFromData will store the data at the right key and base64 encodes it.
func TestAddToSecretFromDataBase64EncodesInput(t *testing.T) {
	t.Parallel()

	contents := random.UniqueId()
	secret := PrepareSecret(
		"test-namespace",
		"test-name",
		map[string]string{},
		map[string]string{},
	)
	AddToSecretFromData(secret, "data", []byte(contents))
	assert.Equal(t, secret.Data["data"], []byte(contents))
}

// Test that we can create secrets that have data added to it from a file
func TestCreateSecretWithDataFromFile(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	// Create a namespace so we don't collide with other tests
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	// Create a dummy secret from a random tmp file
	tmpfile, contents := createRandomFile(t)
	secret := PrepareSecret(
		namespace,
		"secret-for-test",
		map[string]string{},
		map[string]string{},
	)
	require.NoError(t, AddToSecretFromFile(secret, "data", tmpfile))
	require.NoError(t, CreateSecret(kubectlOptions, secret))

	// Now verify the secret was actually created on the cluster.
	// We use the terratest secret lib instead of the one in kubectl.
	ttKubectlOptions.Namespace = namespace
	storedSecret := k8s.GetSecret(t, ttKubectlOptions, "secret-for-test")
	assert.Equal(t, string(storedSecret.Data["data"]), contents)
}

// Test that we can create secrets that have data added to it from a raw bytes
func TestCreateSecretWithDataFromRawBytes(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	// Create a namespace so we don't collide with other tests
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	// Create a dummy secret from a random tmp file
	contents := random.UniqueId()
	secret := PrepareSecret(
		namespace,
		"secret-for-test",
		map[string]string{},
		map[string]string{},
	)
	AddToSecretFromData(secret, "data", []byte(contents))
	require.NoError(t, CreateSecret(kubectlOptions, secret))

	// Now verify the secret was actually created on the cluster.
	// We use the terratest secret lib instead of the one in kubectl.
	ttKubectlOptions.Namespace = namespace
	storedSecret := k8s.GetSecret(t, ttKubectlOptions, "secret-for-test")
	assert.Equal(t, string(storedSecret.Data["data"]), contents)
}

// Test that we can create secrets that have data from multiple sources
func TestCreateSecretWithDataFromMultipleSources(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	// Create a namespace so we don't collide with other tests
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	secret := PrepareSecret(
		namespace,
		"secret-for-test",
		map[string]string{},
		map[string]string{},
	)

	// Create a dummy secret with data from both file and raw bytes
	rawDataContents := random.UniqueId()
	AddToSecretFromData(secret, "rawBytes", []byte(rawDataContents))
	tmpfile, fileContents := createRandomFile(t)
	require.NoError(t, AddToSecretFromFile(secret, "fileData", tmpfile))
	require.NoError(t, CreateSecret(kubectlOptions, secret))

	// Now verify the secret was actually created on the cluster.
	// We use the terratest secret lib instead of the one in kubectl.
	ttKubectlOptions.Namespace = namespace
	storedSecret := k8s.GetSecret(t, ttKubectlOptions, "secret-for-test")

	assert.Equal(t, string(storedSecret.Data["fileData"]), fileContents)
	assert.Equal(t, string(storedSecret.Data["rawBytes"]), rawDataContents)
}

func TestListSecretsOnEmptyReturnsEmptyListWithNoError(t *testing.T) {
	t.Parallel()

	_, kubectlOptions := getKubectlOptions(t)
	namespace := strings.ToLower(random.UniqueId())
	secrets, err := ListSecrets(kubectlOptions, namespace, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, len(secrets), 0)
}

func TestListSecretsShowsSecretInNamespace(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	namespace := strings.ToLower(random.UniqueId())
	configData := createSecret(t, ttKubectlOptions, namespace)
	defer k8s.KubectlDeleteFromString(t, ttKubectlOptions, configData)

	otherNamespace := strings.ToLower(random.UniqueId())
	otherConfigData := createSecret(t, ttKubectlOptions, otherNamespace)
	defer k8s.KubectlDeleteFromString(t, ttKubectlOptions, otherConfigData)

	secrets, err := ListSecrets(kubectlOptions, namespace, metav1.ListOptions{})
	require.NoError(t, err)
	// There is a default service account created in the namespace with a secret token, so there are two
	require.Equal(t, len(secrets), 2)
	found := false
	for _, secret := range secrets {
		if secret.Name == fmt.Sprintf("%s-master-password", namespace) {
			found = true
		}
	}
	assert.True(t, found)
}

func TestGetSecretGetsSecretByName(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	namespace := strings.ToLower(random.UniqueId())
	configData := createSecret(t, ttKubectlOptions, namespace)
	defer k8s.KubectlDeleteFromString(t, ttKubectlOptions, configData)

	secretName := fmt.Sprintf("%s-master-password", namespace)
	secret, err := GetSecret(kubectlOptions, namespace, secretName)
	require.NoError(t, err)
	assert.Equal(t, secret.Name, secretName)
	assert.Equal(t, secret.Namespace, namespace)
}

func TestLabelsSupportForSecrets(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	// Create a namespace so we don't collide with other tests
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	// Create a dummy secret labeled as dummy, and another one that is not labeled
	dummyContents := random.UniqueId()
	secret := PrepareSecret(
		namespace,
		"secret-for-test",
		map[string]string{"type": "dummy"},
		map[string]string{},
	)
	AddToSecretFromData(secret, "data", []byte(dummyContents))
	require.NoError(t, CreateSecret(kubectlOptions, secret))
	otherContents := random.UniqueId()
	otherSecret := PrepareSecret(
		namespace,
		"other-secret-for-test",
		map[string]string{},
		map[string]string{},
	)
	AddToSecretFromData(otherSecret, "data", []byte(otherContents))
	require.NoError(t, CreateSecret(kubectlOptions, otherSecret))

	// Now list the secrets with the filter to make sure labels are applied correctly (and filtering works)
	secrets, err := ListSecrets(kubectlOptions, namespace, metav1.ListOptions{LabelSelector: "type=dummy"})
	require.NoError(t, err)
	require.Equal(t, len(secrets), 1)
	require.Equal(t, secrets[0].Name, "secret-for-test")
	require.Equal(t, string(secrets[0].Data["data"]), dummyContents)
}

func TestDeleteSecret(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	namespace := strings.ToLower(random.UniqueId())
	configData := createSecret(t, ttKubectlOptions, namespace)
	// We use the E version, because this is expected to error out since the secret is removed.
	defer k8s.KubectlDeleteFromStringE(t, ttKubectlOptions, configData)
	secretName := fmt.Sprintf("%s-master-password", namespace)

	// Make sure the secret was created
	ttKubectlOptions.Namespace = namespace
	k8s.GetSecret(t, ttKubectlOptions, secretName)

	// now delete, and make sure the secret can't be obtained
	require.NoError(t, DeleteSecret(kubectlOptions, namespace, secretName))
	_, err := k8s.GetSecretE(t, ttKubectlOptions, secretName)
	require.Error(t, err)
}

// Utility functions used in the tests in this file

func createSecret(t *testing.T, options *k8s.KubectlOptions, namespace string) string {
	configData := fmt.Sprintf(EXAMPLE_SECRET_YAML_TEMPLATE, namespace, namespace, namespace)
	k8s.KubectlApplyFromString(t, options, configData)
	return configData
}

func createRandomFile(t *testing.T) (string, string) {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()
	contents := random.UniqueId()
	tmpfile.WriteString(contents)
	return tmpfile.Name(), contents
}

const EXAMPLE_SECRET_YAML_TEMPLATE = `---
apiVersion: v1
kind: Namespace
metadata:
  name: %s
---
apiVersion: v1
kind: Secret
metadata:
  name: %s-master-password
  namespace: %s
`
