package helm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/helm/pkg/helm/portforwarder"

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
	kubectlOptions := kubectl.GetTestKubectlOptions(t)

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

// This is an end to end integration for the commands to setup helm access. This integration test is designed this way
// due to the way each step is setup to build on the previous step. For example, it is impossible to test grant without
// having a helm server deployed, and configure without running grant.
//
// Test that we can:
// 1. Generate certificate key pairs for use with Tiller
// 2. Upload certificate key pairs to Kubernetes secrets
// 3. Deploy Helm with TLS enabled in the specified namespace
// 4. Grant access to helm
// 5. Configure helm client
// 6. Deploy a helm chart
// 7. Revoke permissions
// 8. Undeploy helm
func TestHelmDeployConfigureUndeploy(t *testing.T) {
	t.Parallel()

	imageSpec := "gcr.io/kubernetes-helm/tiller:v2.14.0"

	kubectlOptions := kubectl.GetTestKubectlOptions(t)
	terratestKubectlOptions := k8s.NewKubectlOptions("", "")
	kubeClient, err := k8s.GetKubernetesClientFromOptionsE(t, terratestKubectlOptions)
	assert.NoError(t, err)
	tlsOptions := tls.SampleTlsOptions(tls.ECDSAAlgorithm)
	clientTLSOptions := tls.SampleTlsOptions(tls.ECDSAAlgorithm)
	clientTLSOptions.DistinguishedName.CommonName = "client"
	namespaceName := strings.ToLower(random.UniqueId())
	serviceAccountName := fmt.Sprintf("%s-service-account", namespaceName)

	defer k8s.DeleteNamespace(t, terratestKubectlOptions, namespaceName)
	k8s.CreateNamespace(t, terratestKubectlOptions, namespaceName)
	terratestKubectlOptions.Namespace = namespaceName

	// Create a test service account we can use for auth
	testServiceAccountName, testServiceAccountKubectlOptions := createServiceAccountForAuth(t, terratestKubectlOptions)
	defer k8s.DeleteConfigContextE(t, testServiceAccountKubectlOptions.ContextName)
	testServiceAccountInfo := ServiceAccountInfo{Name: testServiceAccountName, Namespace: terratestKubectlOptions.Namespace}

	// Create a service account for Tiller
	k8s.CreateServiceAccount(t, terratestKubectlOptions, serviceAccountName)
	bindNamespaceAdminRole(t, terratestKubectlOptions, serviceAccountName)

	defer func() {
		// Make sure to undeploy all helm releases before undeploying the server. However, don't force undeploy the
		// server so that it crashes should the release removal fail.
		assert.NoError(t, Undeploy(kubectlOptions, namespaceName, "", false, true))
	}()
	// Deploy, Grant, and Configure
	assert.NoError(t, Deploy(
		kubectlOptions,
		namespaceName,
		namespaceName,
		serviceAccountName,
		tlsOptions,
		clientTLSOptions,
		getHelmHome(t),
		testServiceAccountInfo,
		imageSpec,
	))

	// Check tiller pod is in chosen namespace
	tillerPodName := validateTillerPodDeployedInNamespace(t, terratestKubectlOptions)

	// Check tiller pod is using the right image
	validateTillerPodImage(t, terratestKubectlOptions, namespaceName, imageSpec)

	// Check tiller pod is launched with the right service account
	validateTillerPodServiceAccount(t, terratestKubectlOptions, tillerPodName, serviceAccountName)

	// Check tiller pod uses secrets instead of configmap as metadata backend
	validateTillerPodUsesSecrets(t, terratestKubectlOptions, tillerPodName)

	// Check tiller pod TLS
	validateTillerPodUsesTLS(t, terratestKubectlOptions)

	// Check tiller pod TLS is different from client TLS
	validateTillerAndClientTLSDifferent(t, terratestKubectlOptions, testServiceAccountInfo)

	// Check that we can deploy a helm chart
	validateHelmChartDeploy(t, testServiceAccountKubectlOptions, namespaceName)

	// Check that the rendered helm env file works
	validateHelmEnvFile(t, testServiceAccountKubectlOptions)

	// Revoke the tiller service account
	rbacGroups := []string{}
	rbacUsers := []string{}
	serviceAccounts := []string{fmt.Sprintf("%s/%s", namespaceName, testServiceAccountName)}
	require.NoError(t, RevokeAccess(kubectlOptions, namespaceName, rbacGroups, rbacUsers, serviceAccounts))

	// ServiceAccount role has been removed
	err = validateNoRole(t, kubeClient, namespaceName, testServiceAccountName)
	roleName := getTillerAccessRoleName(testServiceAccountName, namespaceName)
	assert.Equal(t, err.Error(), fmt.Sprintf("roles.rbac.authorization.k8s.io \"%s\" not found", roleName))

	// ServiceAccount role binding has been removed
	err = validateNoRoleBinding(t, kubeClient, namespaceName, testServiceAccountName)
	roleBindingName := getTillerAccessRoleBindingName(testServiceAccountName, roleName)
	assert.Equal(t, err.Error(), fmt.Sprintf("rolebindings.rbac.authorization.k8s.io \"%s\" not found", roleBindingName))

	// ServiceAccount TLS secret has been removed
	err = validateNoTLSSecret(t, kubeClient, namespaceName, serviceAccountName)
	secretName := getTillerClientCertSecretName(serviceAccountName)
	assert.Equal(t, err.Error(), fmt.Sprintf("secrets \"%s\" not found", secretName))
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

// validateTillerPodImage checks if the deployed tiller image is actually the one we configured.
func validateTillerPodImage(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions, tillerNamespace string, tillerImageSpec string) {
	kubeClient, err := k8s.GetKubernetesClientFromOptionsE(t, terratestKubectlOptions)
	require.NoError(t, err)
	image, err := portforwarder.GetTillerPodImage(kubeClient.CoreV1(), tillerNamespace)
	require.NoError(t, err)
	assert.Equal(t, image, tillerImageSpec)
}

// validateTillerPodDeployedInNamespace validates that the tiller pod was deployed with the provided service account
func validateTillerPodServiceAccount(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions, tillerPodName string, serviceAccountName string) {
	pod := k8s.GetPod(t, terratestKubectlOptions, tillerPodName)
	assert.Equal(t, pod.Spec.ServiceAccountName, serviceAccountName)
}

// validateTillerPodUsesSecrets validates that the tiller pod is deployed with using secrets for metadata instead of
// configmaps.
func validateTillerPodUsesSecrets(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions, tillerPodName string) {
	// Check the boot logs to make sure tiller is using Secrets as the storage driver
	out, err := k8s.RunKubectlAndGetOutputE(t, terratestKubectlOptions, "logs", tillerPodName)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(out, "Storage driver is Secret"))
}

// validateTillerPodUsesTLS verifies that the deployed tiller pod has TLS certs configured.
func validateTillerPodUsesTLS(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions) {
	secret := k8s.GetSecret(t, terratestKubectlOptions, "tiller-secret")
	for _, expectedKey := range []string{"tls.key", "ca.crt", "tls.crt"} {
		_, hasKey := secret.Data[expectedKey]
		assert.True(t, hasKey)
	}
}

// validateTillerAndClientTLSDifferent verifies that the TLS cert generated for the client is different from that
// generated for the server.
func validateTillerAndClientTLSDifferent(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions, serviceAccountInfo ServiceAccountInfo) {
	clientCertSecretName := getTillerClientCertSecretName(serviceAccountInfo.EntityID())
	clientSecret := k8s.GetSecret(t, terratestKubectlOptions, clientCertSecretName)
	clientCert := clientSecret.Data["client.crt"]
	clientCertSubject := getCertificateSubjectInfoFromBytes(t, clientCert)

	serverSecret := k8s.GetSecret(t, terratestKubectlOptions, "tiller-secret")
	tillerCert := serverSecret.Data["tls.crt"]
	tillerCertSubject := getCertificateSubjectInfoFromBytes(t, tillerCert)

	assert.NotEqual(t, clientCertSubject, tillerCertSubject)
}

func validateGenerateCertificateKeyPair(t *testing.T, algorithm string) {
	tmpDir, err := ioutil.TempDir("", algorithm)
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tlsOptions := tls.SampleTlsOptions(algorithm)
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

// validateHelmChartDeploy checks if we can deploy a simple helm chart to the server.
func validateHelmChartDeploy(t *testing.T, kubectlOptions *kubectl.KubectlOptions, namespace string) {
	require.NoError(
		t,
		RunHelm(
			kubectlOptions,
			"install",
			"stable/kubernetes-dashboard",
			"--wait",
			"--tls",
			"--tls-verify",
			"--tiller-namespace",
			namespace,
			"--namespace",
			namespace,
		),
	)
}

// validateHelmEnvFile sources the generated helm env file and verifies it sets the necessary and sufficient
// environment variables for helm to talk to the deployed Tiller instance.
func validateHelmEnvFile(t *testing.T, options *kubectl.KubectlOptions) {
	helmArgs := []string{"helm"}
	if options.ContextName != "" {
		helmArgs = append(helmArgs, "--kube-context", options.ContextName)
	}
	if options.ConfigPath != "" {
		helmArgs = append(helmArgs, "--kubeconfig", options.ConfigPath)
	}
	helmArgs = append(helmArgs, "ls")
	helmCmd := strings.Join(helmArgs, " ")

	helmEnvPath := filepath.Join(getHelmHome(t), envFileName)
	// TODO: make this test platform independent
	cmd := shell.Command{
		Command: "sh",
		Args: []string{
			"-c",
			fmt.Sprintf(". %s && %s", helmEnvPath, helmCmd),
		},
	}
	shell.RunCommand(t, cmd)
}

// validateServiceAccountRoleRemoved
func validateNoRole(t *testing.T, client *kubernetes.Clientset, namespace, serviceAccountName string) error {
	roleName := getTillerAccessRoleName(serviceAccountName, namespace)
	_, err := client.RbacV1().Roles(namespace).Get(roleName, metav1.GetOptions{})
	return err
}

// validateNoServiceAccountRoleBinding validates that the mock service account does not have an associated rolebinding
func validateNoRoleBinding(t *testing.T, client *kubernetes.Clientset, namespace, serviceAccountName string) error {
	roleName := getTillerAccessRoleName(serviceAccountName, namespace)
	roleBindingName := getTillerAccessRoleBindingName(serviceAccountName, roleName)
	_, err := client.RbacV1().RoleBindings(namespace).Get(roleBindingName, metav1.GetOptions{})
	return err
}

// validateNoTLSSecret validates that the mock service account does not have an associated secret TLS keypair
func validateNoTLSSecret(t *testing.T, client *kubernetes.Clientset, namespace, serviceAccountName string) error {
	secretName := getTillerClientCertSecretName(serviceAccountName)
	_, err := client.CoreV1().Secrets(namespace).Get(secretName, metav1.GetOptions{})
	return err
}
