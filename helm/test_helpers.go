package helm

import (
	"crypto/x509/pkix"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/logger"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/tls"
)

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

func getHelmHome(t *testing.T) string {
	helmHome, err := GetDefaultHelmHome()
	require.NoError(t, err)
	return helmHome
}

func bindNamespaceAdminRole(t *testing.T, ttKubectlOptions *k8s.KubectlOptions, serviceAccountName string) {
	clientset, err := k8s.GetKubernetesClientFromOptionsE(t, ttKubectlOptions)
	require.NoError(t, err)

	// Create the admin rbac role
	role := rbacv1.Role{
		Rules: []rbacv1.PolicyRule{
			rbacv1.PolicyRule{
				Verbs:     []string{"*"},
				APIGroups: []string{"*"},
				Resources: []string{"*"},
			},
		},
	}
	role.Name = fmt.Sprintf("%s-admin-%s", ttKubectlOptions.Namespace, random.UniqueId())
	role.Namespace = ttKubectlOptions.Namespace
	_, err = clientset.RbacV1().Roles(ttKubectlOptions.Namespace).Create(&role)
	require.NoError(t, err)

	// ... and bind it to the service account
	binding := rbacv1.RoleBinding{
		Subjects: []rbacv1.Subject{
			rbacv1.Subject{
				Kind:      "ServiceAccount",
				APIGroup:  "",
				Name:      serviceAccountName,
				Namespace: ttKubectlOptions.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
	}
	binding.Name = fmt.Sprintf("%s-admin-binding-%s", serviceAccountName, random.UniqueId())
	_, err = clientset.RbacV1().RoleBindings(ttKubectlOptions.Namespace).Create(&binding)
	require.NoError(t, err)
}

func createServiceAccountForAuth(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions) (string, *kubectl.KubectlOptions) {
	contextName := random.UniqueId()
	serviceAccountName := strings.ToLower(random.UniqueId())

	// Create a new service account that we will use for auth.
	// We intentionally bind no role to the account to test the granting process, which will grant enough permissions to
	// configure and access helm.
	k8s.CreateServiceAccount(t, terratestKubectlOptions, serviceAccountName)

	// Add a new context with the service account as auth
	// First wait for the TokenController to provision a ServiceAccount token
	msg := retry.DoWithRetry(
		t,
		"Waiting for ServiceAccount Token to be provisioned",
		30,
		10*time.Second,
		func() (string, error) {
			logger.Logf(t, "Checking if service account has secret")
			serviceAccount := k8s.GetServiceAccount(t, terratestKubectlOptions, serviceAccountName)
			if len(serviceAccount.Secrets) == 0 {
				msg := "No secrets on the service account yet"
				logger.Logf(t, msg)
				return "", fmt.Errorf(msg)
			}
			return "Service Account has secret", nil
		},
	)
	logger.Logf(t, msg)
	// Then get the service account token
	serviceAccount := k8s.GetServiceAccount(t, terratestKubectlOptions, serviceAccountName)
	require.Equal(t, len(serviceAccount.Secrets), 1)
	secret := k8s.GetSecret(t, terratestKubectlOptions, serviceAccount.Secrets[0].Name)
	// Then update config to include the service account token
	k8s.RunKubectl(
		t,
		terratestKubectlOptions,
		"config",
		"set-credentials",
		serviceAccountName,
		"--token",
		string(secret.Data["token"]),
	)
	// Next extract the currently configured cluster
	var configPath string
	if terratestKubectlOptions.ConfigPath != "" {
		configPath = terratestKubectlOptions.ConfigPath
	} else {
		defaultConfigPath, err := k8s.GetKubeConfigPathE(t)
		require.NoError(t, err)
		configPath = defaultConfigPath
	}
	config := k8s.LoadConfigFromPath(configPath)
	rawConfig, err := config.RawConfig()
	require.NoError(t, err)
	cluster := rawConfig.Contexts[rawConfig.CurrentContext].Cluster
	// Afterwards create a new config context binding the cluster to the service account
	k8s.RunKubectl(
		t,
		terratestKubectlOptions,
		"config",
		"set-context",
		contextName,
		"--cluster",
		cluster,
		"--user",
		serviceAccountName,
	)
	// Finally, create a new KubectlOption that can be used in the context
	return serviceAccountName, kubectl.NewKubectlOptions(contextName, configPath)
}

// copyKubeconfigToTempFile will copy the default kubeconfig to a temp file that can be used to test config manipulation
// in isolation.
func copyKubeconfigToTempFile(t *testing.T) string {
	kubeConfigPath, err := k8s.GetKubeConfigPathE(t)
	require.NoError(t, err)
	data, err := ioutil.ReadFile(kubeConfigPath)
	require.NoError(t, err)
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()
	_, err = tmpfile.Write(data)
	require.NoError(t, err)
	return tmpfile.Name()
}
