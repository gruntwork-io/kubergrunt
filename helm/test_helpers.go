package helm

import (
	"crypto/x509/pkix"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/logger"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/retry"
	homedir "github.com/mitchellh/go-homedir"
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

func grantAndConfigureClientAsServiceAccount(
	t *testing.T,
	terratestKubectlOptions *k8s.KubectlOptions,
	kubectlOptions *kubectl.KubectlOptions,
	tlsOptions tls.TLSOptions,
) *kubectl.KubectlOptions {
	serviceAccountName, serviceAccountKubectlOptions := createServiceAccountForAuth(t, terratestKubectlOptions)

	// Grant access to the helm client to the service account
	require.NoError(t, GrantAccess(
		kubectlOptions,
		tlsOptions,
		terratestKubectlOptions.Namespace,
		[]string{},
		[]ServiceAccountInfo{
			ServiceAccountInfo{
				Name:      serviceAccountName,
				Namespace: terratestKubectlOptions.Namespace,
			},
		},
	))

	// TODO: Temporary hack to configure the helm client. In the near future, this should be replaced with the
	//       configure command
	configureHelmClient(t, terratestKubectlOptions, terratestKubectlOptions.Namespace, serviceAccountName)
	return serviceAccountKubectlOptions
}

// configureHelmClient is a temporary hack to configure the local helm client to be able to communicate with the
// deployed helm server. This hack will simply reuse the tiller certs.
func configureHelmClient(t *testing.T, terratestKubectlOptions *k8s.KubectlOptions, namespaceName string, serviceAccountName string) {
	secretName := fmt.Sprintf("%s-namespace-%s-client-certs", namespaceName, serviceAccountName)
	secret := k8s.GetSecret(t, terratestKubectlOptions, secretName)
	tillerSecret := k8s.GetSecret(t, terratestKubectlOptions, "tiller-secret")
	helmHome := getHelmHome(t)
	decodedData := tillerSecret.Data["ca.crt"]
	require.NoError(t, ioutil.WriteFile(filepath.Join(helmHome, "ca.pem"), decodedData, 0644))

	decodedData = secret.Data["client.pem"]
	require.NoError(t, ioutil.WriteFile(filepath.Join(helmHome, "key.pem"), decodedData, 0644))

	decodedData = secret.Data["client.crt"]
	require.NoError(t, ioutil.WriteFile(filepath.Join(helmHome, "cert.pem"), decodedData, 0644))
}

func getHelmHome(t *testing.T) string {
	home, err := homedir.Dir()
	require.NoError(t, err)
	helmHome := filepath.Join(home, ".helm")
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

	// Create a new admin service account that we will use
	k8s.CreateServiceAccount(t, terratestKubectlOptions, serviceAccountName)
	bindNamespaceAdminRole(t, terratestKubectlOptions, serviceAccountName)

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
