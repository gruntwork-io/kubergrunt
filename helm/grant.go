package helm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
	"github.com/gruntwork-io/kubergrunt/tls"
)

type RBACType int

const (
	Group RBACType = iota
	ServiceAccount
)

type ServiceAccountInfo struct {
	Name      string
	Namespace string
}

func (serviceAccountInfo ServiceAccountInfo) String() string {
	return fmt.Sprintf("%s/%s", serviceAccountInfo.Namespace, serviceAccountInfo.Name)
}

// GrantAccess grants the provided RBAC groups and/or service accounts access to the Tiller Pod available in the
// provided Tiller namespace.
// Specifically, this will:
// - Download the corresponding CA keypair for the Tiller deployment from Kubernetes.
// - Issue a new TLS certificate keypair using the CA keypair.
// - Upload the new TLS certificate keypair to a new Secret in the Tiller namespace.
// - Create a new RBAC role that grants read only pod access to the Tiller namespace, and read only access to the Secret
//   containing the TLS certificate keypair.
// - Remove the local copies of the downloaded and generated certificates.
func GrantAccess(
	kubectlOptions *kubectl.KubectlOptions,
	tlsOptions tls.TLSOptions,
	tillerNamespace string,
	rbacGroups []string,
	serviceAccounts []ServiceAccountInfo,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof(
		"Granting access Tiller server deployed in namespace %s to the RBAC groups %v and service accounts %v.",
		tillerNamespace, rbacGroups, serviceAccounts,
	)

	logger.Info("Checking if Tiller is deployed in the namespace.")
	if err := validateTillerDeployed(kubectlOptions, tillerNamespace); err != nil {
		logger.Errorf("Did not find a deployed Tiller instance in the namespace %s.", tillerNamespace)
		return err
	}
	logger.Infof("Found a valid Tiller instance in the namespace %s.", tillerNamespace)

	logger.Infof("Downloading the CA TLS certificates for Tiller deployed in namespace %s.", tillerNamespace)
	tlsPath, err := ioutil.TempDir("", "")
	if err != nil {
		logger.Errorf("Error creating temp directory to store certificate key pairs: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Using %s as temp path for storing certificates", tlsPath)
	defer os.RemoveAll(tlsPath)
	caKeyPairPath, err := downloadCATLSCertificates(kubectlOptions, tillerNamespace, tlsPath)
	if err != nil {
		logger.Errorf("Error downloading the CA TLS certificates for Tiller deployed in namespace %s.", tillerNamespace)
		return err
	}
	logger.Infof("Successfully downloaded CA TLS certificates for Tiller deployed in namespace %s.", tillerNamespace)

	logger.Infof("Granting access to deployed Tiller in namespace %s to RBAC groups", tillerNamespace)
	if err := grantAccessToRBACGroups(kubectlOptions, tlsOptions, caKeyPairPath, tillerNamespace, rbacGroups); err != nil {
		logger.Errorf("Error granting access to deployed Tiller in namespace %s to RBAC groups: %s", tillerNamespace, err)
		return err
	}
	logger.Infof("Successfully granted access to deployed Tiller in namespace %s to RBAC groups", tillerNamespace)

	logger.Infof("Granting access to deployed Tiller in namespace %s to Service Accounts", tillerNamespace)
	if err := grantAccessToServiceAccounts(kubectlOptions, tlsOptions, caKeyPairPath, tillerNamespace, serviceAccounts); err != nil {
		logger.Errorf("Error granting access to deployed Tiller in namespace %s to Service Accounts: %s", tillerNamespace, err)
		return err
	}
	logger.Infof("Successfully granted access to deployed Tiller in namespace %s to Service Accounts", tillerNamespace)

	logger.Infof(
		"Successfully granted access to deployed Tiller server in namespace %s to the RBAC groups %v and service accounts %v.",
		tillerNamespace, rbacGroups, serviceAccounts,
	)

	// Print follow up instructions
	fmt.Printf("\n%s\n", fmt.Sprintf(Instructions, tillerNamespace, tillerNamespace))
	return nil
}

// validateTillerDeployed will look for a valid Tiller instance deployed into the provided namespace.
// We do this by looking for a pod with the labels "app=helm" and "name=tiller", which are the annotations given to the
// Tiller pod by helm.
func validateTillerDeployed(kubectlOptions *kubectl.KubectlOptions, tillerNamespace string) error {
	filters := metav1.ListOptions{LabelSelector: "app=helm,name=tiller"}
	pods, err := kubectl.ListPods(kubectlOptions, tillerNamespace, filters)
	if err != nil {
		return err
	}
	if len(pods) == 0 {
		msg := fmt.Sprintf("Could not find valid Tiller deployment in namespace %s", tillerNamespace)
		return errors.WithStackTrace(HelmValidationError{msg})
	}
	return nil
}

// grantAccessToRBACGroups will grant access to the deployed Tiller server to the provided RBAC groups.
func grantAccessToRBACGroups(
	kubectlOptions *kubectl.KubectlOptions,
	tlsOptions tls.TLSOptions,
	caKeyPairPath tls.CertificateKeyPairPath,
	tillerNamespace string,
	rbacGroups []string,
) error {
	logger := logging.GetProjectLogger()

	numGroups := len(rbacGroups)
	for idx, rbacGroup := range rbacGroups {
		logger.Infof("Generating and storing certificate key pair for group %s (%d of %d)", rbacGroup, idx+1, numGroups)
		clientSecretName, err := generateAndStoreSignedCertificateKeyPair(kubectlOptions, tlsOptions, caKeyPairPath, tillerNamespace, Group, rbacGroup)
		if err != nil {
			logger.Errorf("Error generating and storing certificate key pair for group %s", rbacGroup)
			return err
		}
		logger.Infof("Successfully generated and stored certificate key pair for group %s", rbacGroup)

		logger.Infof("Creating and binding RBAC roles to group %s", rbacGroup)
		err = createAndBindRBACRolesForTillerAccess(kubectlOptions, tillerNamespace, clientSecretName, Group, rbacGroup)
		if err != nil {
			logger.Errorf("Error creating and binding RBAC roles to group %s", rbacGroup)
			return err
		}
		logger.Infof("Successfully bound RBAC roles to group %s", rbacGroup)
	}
	return nil
}

// grantAccessToServiceAccounts will grant access to the deployed Tiller server to the provided service accounts.
func grantAccessToServiceAccounts(
	kubectlOptions *kubectl.KubectlOptions,
	tlsOptions tls.TLSOptions,
	caKeyPairPath tls.CertificateKeyPairPath,
	tillerNamespace string,
	serviceAccounts []ServiceAccountInfo,
) error {
	logger := logging.GetProjectLogger()

	numAccounts := len(serviceAccounts)
	for idx, serviceAccount := range serviceAccounts {
		logger.Infof("Generating and storing certificate key pair for service account %s (%d of %d)", serviceAccount, idx+1, numAccounts)
		clientSecretName, err := generateAndStoreSignedCertificateKeyPair(kubectlOptions, tlsOptions, caKeyPairPath, tillerNamespace, ServiceAccount, serviceAccount)
		if err != nil {
			logger.Errorf("Error generating and storing certificate key pair for service account %s", serviceAccount)
			return err
		}
		logger.Infof("Successfully generated and stored certificate key pair for service account %s", serviceAccount)

		logger.Infof("Creating and binding RBAC roles to service account %s", serviceAccount)
		err = createAndBindRBACRolesForTillerAccess(kubectlOptions, tillerNamespace, clientSecretName, ServiceAccount, serviceAccount)
		if err != nil {
			logger.Errorf("Error creating and binding RBAC roles to service account %s", serviceAccount)
			return err
		}
		logger.Infof("Successfully bound RBAC roles to service account %s", serviceAccount)
	}
	return nil
}

// downloadCATLSCertificates will download the TLS certificate keypair for the Tiller deployed at the provided
// namespace. This assumes that the CA secrets are stored in the kube-system namespace with the label
// "tiller-namespace=TILLER_NAMESPACE".
func downloadCATLSCertificates(kubectlOptions *kubectl.KubectlOptions, tillerNamespace string, tmpStorePath string) (tls.CertificateKeyPairPath, error) {
	// First get the Secret containing the TLS certificates for the CA for the deployed Tiller.
	secretName := fmt.Sprintf("%s-namespace-ca-certs", tillerNamespace)
	secret, err := kubectl.GetSecret(kubectlOptions, "kube-system", secretName)
	if err != nil {
		return tls.CertificateKeyPairPath{}, err
	}

	// Now store the certificate key pairs on disk into a temporary location.
	certPath := filepath.Join(tmpStorePath, "ca.crt")
	if err := ioutil.WriteFile(certPath, secret.Data["ca.crt"], 0600); err != nil {
		return tls.CertificateKeyPairPath{}, errors.WithStackTrace(err)
	}
	privKeyPath := filepath.Join(tmpStorePath, "ca.pem")
	if err := ioutil.WriteFile(privKeyPath, secret.Data["ca.pem"], 0600); err != nil {
		return tls.CertificateKeyPairPath{}, errors.WithStackTrace(err)
	}
	pubKeyPath := filepath.Join(tmpStorePath, "ca.pub")
	if err := ioutil.WriteFile(pubKeyPath, secret.Data["ca.pub"], 0600); err != nil {
		return tls.CertificateKeyPairPath{}, errors.WithStackTrace(err)
	}

	// Finally build and return the struct
	return tls.CertificateKeyPairPath{
		CertificatePath: certPath,
		PrivateKeyPath:  privKeyPath,
		PublicKeyPath:   pubKeyPath,
	}, nil
}

// generateAndStoreSignedCertificateKeyPair will generate new client side certificates that are signed by the given CA.
// These certs will then be uploaded to a Secret residing in the Tiller namespace.
func generateAndStoreSignedCertificateKeyPair(
	kubectlOptions *kubectl.KubectlOptions,
	tlsOptions tls.TLSOptions,
	caKeyPairPath tls.CertificateKeyPairPath,
	tillerNamespace string,
	entityType RBACType,
	rbacEntity interface{}, // Want to be able to accept group (which is string) or service account (which is ServiceAccountInfo)
) (string, error) {
	logger := logging.GetProjectLogger()

	tlsPath, err := ioutil.TempDir("", "")
	if err != nil {
		logger.Errorf("Error creating temp directory to store client certificate key pairs: %s", err)
		return "", errors.WithStackTrace(err)
	}
	logger.Infof("Using %s as temp path for storing client certificates", tlsPath)
	defer os.RemoveAll(tlsPath)

	logger.Infof("Generating client certificates for entity %s", rbacEntity)
	clientKeyPairPath, err := generateSignedCertificateKeyPair(
		tlsOptions,
		tlsPath,
		caKeyPairPath,
		"client", // all the client certs use client as the name base so it is easy to find
	)
	if err != nil {
		return "", err
	}
	logger.Infof("Successfully generated client certificates for entity %s", rbacEntity)

	var entityKey, entityName, clientSecretName string
	switch entityType {
	case Group:
		entityName = rbacEntity.(string)
		clientSecretName = fmt.Sprintf("%s-namespace-%s-client-certs", tillerNamespace, entityName)
		entityKey = "tiller-client-rbac-group"
	case ServiceAccount:
		entityName = rbacEntity.(ServiceAccountInfo).Name
		clientSecretName = fmt.Sprintf("%s-namespace-%s-client-certs", tillerNamespace, entityName)
		entityKey = "tiller-client-service-account"
	}
	logger.Infof("Uploading client certificate key pair as secret in namespace %s with name %s", tillerNamespace, clientSecretName)
	err = StoreCertificateKeyPairAsKubernetesSecret(
		kubectlOptions,
		clientSecretName,
		tillerNamespace,
		map[string]string{
			"tiller-namespace":          tillerNamespace,
			"tiller-client-credentials": "true",
			entityKey:                   entityName,
		},
		map[string]string{},
		"client",
		clientKeyPairPath,
	)
	if err != nil {
		logger.Errorf("Error uploading client certificate key pair as a secret: %s", err)
		return "", err
	}
	logger.Info("Successfully uploaded client certificate key pair as a secret")
	return clientSecretName, nil
}

// createAndBindRBACRolesForTillerAccess will create RBAC roles that grant:
// - Get and List access to pods in the tiller namespace (be able to look up and connect to the Tiller server)
// - Get the client TLS certificate Secret resource in the tiller namespace.
func createAndBindRBACRolesForTillerAccess(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	clientSecretName string,
	entityType RBACType,
	rbacEntity interface{}, // Want to be able to accept group (which is string) or service account (which is ServiceAccountInfo)
) error {
	logger := logging.GetProjectLogger()

	client, err := kubectl.GetKubernetesClientFromFile(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	if err != nil {
		return err
	}

	logger.Infof("Creating RBAC role to grant access to Tiller in namespace %s to %s", tillerNamespace, rbacEntity)
	rbacRole := rbacv1.Role{
		Rules: []rbacv1.PolicyRule{
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list"},
				APIGroups: []string{""},
				Resources: []string{"pods", fmt.Sprintf("secrets/%s", clientSecretName)},
			},
		},
	}
	switch entityType {
	case Group:
		rbacRole.Name = fmt.Sprintf("%s-%s-tiller-access", rbacEntity, tillerNamespace)
	case ServiceAccount:
		// We can't have slashes in the name
		rbacRole.Name = fmt.Sprintf("%s-%s-tiller-access", rbacEntity.(ServiceAccountInfo).Name, tillerNamespace)
	}
	rbacRole.Namespace = tillerNamespace
	_, err = client.RbacV1().Roles(tillerNamespace).Create(&rbacRole)
	if err != nil {
		logger.Errorf("Error creating RBAC role to grant access to Tiller: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully created RBAC role %s", rbacRole.Name)

	logger.Infof("Creating binding for role %s to %s", rbacRole.Name, rbacEntity)
	var subject rbacv1.Subject
	switch entityType {
	case Group:
		subject = rbacv1.Subject{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Group",
			Name:     rbacEntity.(string),
		}
	case ServiceAccount:
		serviceAccountInfo := rbacEntity.(ServiceAccountInfo)
		subject = rbacv1.Subject{
			APIGroup:  "",
			Kind:      "ServiceAccount",
			Name:      serviceAccountInfo.Name,
			Namespace: serviceAccountInfo.Namespace,
		}
	}
	binding := rbacv1.RoleBinding{
		Subjects: []rbacv1.Subject{subject},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     rbacRole.Name,
		},
	}
	binding.Name = fmt.Sprintf("%s-%s-binding", subject.Name, rbacRole.Name)
	binding.Namespace = tillerNamespace
	_, err = client.RbacV1().RoleBindings(tillerNamespace).Create(&binding)
	if err != nil {
		logger.Errorf("Error binding RBAC role %s to %s: %s", rbacRole.Name, rbacEntity, err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully bound role %s to %s", rbacRole.Name, rbacEntity)

	return nil
}

const Instructions = `Your users should now be able to setup their local helm client to access Tiller now. To configure their client, they should use the "kubergrunt helm configure" command:

   kubergrunt helm configure --tiller-namespace %s

If they wish to further setup kubectl to default to the managed namespace, they can pass in the following options:

   kubergrunt helm configure \
     --tiller-namespace %s \
	 --resource-namespace RESOURCE_NAMESPACE \
	 --set-kubectl-namespace
`
