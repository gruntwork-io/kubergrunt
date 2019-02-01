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

// GrantAccess grants the provided RBAC groups and/or service accounts access to the Tiller Pod available in the
// provided Tiller namespace.
// Specifically, this will:
// - Download the corresponding CA keypair for the Tiller deployment from Kubernetes. Assumes the CA cert is in the
//   kube-system namespace.
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
	rbacUsers []string,
	serviceAccounts []string,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof(
		"Granting access Tiller server deployed in namespace %s to:",
		tillerNamespace,
	)
	logEntities(rbacGroups, rbacUsers, serviceAccounts)

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
	if err := grantAccessToRBACEntities(kubectlOptions, tlsOptions, caKeyPairPath, tillerNamespace, convertToGroupInfos(rbacGroups)); err != nil {
		logger.Errorf("Error granting access to deployed Tiller in namespace %s to RBAC groups: %s", tillerNamespace, err)
		return err
	}
	logger.Infof("Successfully granted access to deployed Tiller in namespace %s to RBAC groups", tillerNamespace)

	logger.Infof("Granting access to deployed Tiller in namespace %s to RBAC users", tillerNamespace)
	if err := grantAccessToRBACEntities(kubectlOptions, tlsOptions, caKeyPairPath, tillerNamespace, convertToUserInfos(rbacUsers)); err != nil {
		logger.Errorf("Error granting access to deployed Tiller in namespace %s to RBAC users: %s", tillerNamespace, err)
		return err
	}
	logger.Infof("Successfully granted access to deployed Tiller in namespace %s to RBAC users", tillerNamespace)

	logger.Infof("Granting access to deployed Tiller in namespace %s to Service Accounts", tillerNamespace)
	serviceAccountInfos, err := convertToServiceAccountInfos(serviceAccounts)
	if err != nil {
		return err
	}
	if err := grantAccessToRBACEntities(kubectlOptions, tlsOptions, caKeyPairPath, tillerNamespace, serviceAccountInfos); err != nil {
		logger.Errorf("Error granting access to deployed Tiller in namespace %s to Service Accounts: %s", tillerNamespace, err)
		return err
	}
	logger.Infof("Successfully granted access to deployed Tiller in namespace %s to Service Accounts", tillerNamespace)

	logger.Infof("Successfully granted access to deployed Tiller server in namespace %s to:", tillerNamespace)
	logEntities(rbacGroups, rbacUsers, serviceAccounts)

	// Print follow up instructions
	fmt.Printf("\n%s\n", fmt.Sprintf(Instructions, tillerNamespace, tillerNamespace))
	return nil
}

func logEntities(rbacGroups []string, rbacUsers []string, serviceAccounts []string) {
	logger := logging.GetProjectLogger()
	if len(rbacGroups) > 0 {
		logger.Infof("\t- the RBAC groups %v", rbacGroups)
	}
	if len(rbacUsers) > 0 {
		logger.Infof("\t- the RBAC users %v", rbacUsers)
	}
	if len(serviceAccounts) > 0 {
		logger.Infof("\t- the service accounts %v", serviceAccounts)
	}
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

// grantAccessToRBACEntities will grant access to the deployed Tiller server to the provided RBAC groups.
// The granting process is as follows:
// For each RBAC group:
// - Create a new signed certificate from the provided CA keypair.
// - Upload the new certificate as a Secret resource to the tiller namespace with sufficient labels such that it can be
//   found later by the configure command.
// - Create a new RBAC role with read pod permissions and read secret permission **for the specific secret** in the
//   tiller namespace, and bind it to the group..
func grantAccessToRBACEntities(
	kubectlOptions *kubectl.KubectlOptions,
	tlsOptions tls.TLSOptions,
	caKeyPairPath tls.CertificateKeyPairPath,
	tillerNamespace string,
	rbacEntities []RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	numEntities := len(rbacEntities)
	for idx, rbacEntity := range rbacEntities {
		logger.Infof("Generating and storing certificate key pair for %s (%d of %d)", rbacEntity, idx+1, numEntities)
		clientSecretName, err := generateAndStoreSignedCertificateKeyPair(kubectlOptions, tlsOptions, caKeyPairPath, tillerNamespace, rbacEntity)
		if err != nil {
			logger.Errorf("Error generating and storing certificate key pair for %s", rbacEntity)
			return err
		}
		logger.Infof("Successfully generated and stored certificate key pair for %s", rbacEntity)

		logger.Infof("Creating and binding RBAC roles to %s", rbacEntity)
		err = createAndBindRBACRolesForTillerAccess(kubectlOptions, tillerNamespace, clientSecretName, rbacEntity)
		if err != nil {
			logger.Errorf("Error creating and binding RBAC roles to %s", rbacEntity)
			return err
		}
		logger.Infof("Successfully bound RBAC roles to %s", rbacEntity)
	}
	return nil
}

// downloadCATLSCertificates will download the TLS certificate keypair for the Tiller deployed at the provided
// namespace. This assumes that the CA secrets are stored in the kube-system namespace with the label
// "tiller-namespace=TILLER_NAMESPACE".
func downloadCATLSCertificates(kubectlOptions *kubectl.KubectlOptions, tillerNamespace string, tmpStorePath string) (tls.CertificateKeyPairPath, error) {
	// First get the Secret containing the TLS certificates for the CA for the deployed Tiller.
	secretName := getTillerCACertSecretName(tillerNamespace)
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
	rbacEntity RBACEntity,
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

	entityKey := fmt.Sprintf("tiller-client-rbac-%s", rbacEntity.EntityType())
	clientSecretName := getTillerClientCertSecretName(rbacEntity.EntityID())
	logger.Infof("Uploading client certificate key pair as secret in kube-system namespace with name %s", clientSecretName)
	err = StoreCertificateKeyPairAsKubernetesSecret(
		kubectlOptions,
		clientSecretName,
		"kube-system",
		map[string]string{
			"gruntwork.io/tiller-namespace":        tillerNamespace,
			"gruntwork.io/tiller-credentials":      "true",
			"gruntwork.io/tiller-credentials-type": "client",
		},
		map[string]string{
			fmt.Sprintf("gruntwork.io/%s", entityKey): rbacEntity.EntityID(),
		},
		"client",
		clientKeyPairPath,
		caKeyPairPath.CertificatePath,
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
	rbacEntity RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	client, err := kubectl.GetKubernetesClientFromFile(kubectlOptions.ConfigPath, kubectlOptions.ContextName)
	if err != nil {
		return err
	}

	entityNameForResourceName := md5HashString(rbacEntity.EntityID())

	logger.Infof("Creating RBAC role to grant access to Tiller in namespace %s to %s", tillerNamespace, rbacEntity)
	rbacRole := rbacv1.Role{
		Rules: []rbacv1.PolicyRule{
			rbacv1.PolicyRule{
				Verbs:     []string{"get", "list"},
				APIGroups: []string{""},
				Resources: []string{"pods"},
			},
			rbacv1.PolicyRule{
				Verbs:     []string{"create"},
				APIGroups: []string{""},
				Resources: []string{"pods/portforward"},
			},
		},
	}
	rbacRole.Name = fmt.Sprintf("%s-%s-tiller-access", entityNameForResourceName, tillerNamespace)
	rbacRole.Namespace = tillerNamespace
	_, err = client.RbacV1().Roles(tillerNamespace).Create(&rbacRole)
	if err != nil {
		logger.Errorf("Error creating RBAC role to grant access to Tiller: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully created RBAC role %s", rbacRole.Name)

	logger.Infof("Creating binding for role %s to %s", rbacRole.Name, rbacEntity)
	binding := rbacv1.RoleBinding{
		Subjects: []rbacv1.Subject{rbacEntity.Subject()},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     rbacRole.Name,
		},
	}
	binding.Name = fmt.Sprintf("%s-binding", rbacRole.Name)
	binding.Namespace = tillerNamespace
	_, err = client.RbacV1().RoleBindings(tillerNamespace).Create(&binding)
	if err != nil {
		logger.Errorf("Error binding RBAC role %s to %s: %s", rbacRole.Name, rbacEntity, err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully bound role %s to %s", rbacRole.Name, rbacEntity)

	logger.Infof("Creating RBAC role to grant access to client certificates for Tiller in namespace %s to %s", tillerNamespace, rbacEntity)
	// Should only have access to the one secret in the kube-system namespace
	certRBACRole := rbacv1.Role{
		Rules: []rbacv1.PolicyRule{
			rbacv1.PolicyRule{
				Verbs:         []string{"get"},
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{clientSecretName},
			},
		},
	}
	certRBACRole.Name = fmt.Sprintf("%s-%s-tiller-access-creds", entityNameForResourceName, tillerNamespace)
	rbacRole.Namespace = "kube-system"
	_, err = client.RbacV1().Roles("kube-system").Create(&certRBACRole)
	if err != nil {
		logger.Errorf("Error creating RBAC role to grant access to client certificates for Tiller: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully created RBAC role %s", certRBACRole.Name)

	logger.Infof("Creating binding for role %s to %s", certRBACRole.Name, rbacEntity)
	certRoleBinding := rbacv1.RoleBinding{
		Subjects: []rbacv1.Subject{rbacEntity.Subject()},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     certRBACRole.Name,
		},
	}
	certRoleBinding.Name = fmt.Sprintf("%s-binding", certRBACRole.Name)
	certRoleBinding.Namespace = "kube-system"
	_, err = client.RbacV1().RoleBindings("kube-system").Create(&certRoleBinding)
	if err != nil {
		logger.Errorf("Error binding RBAC role %s to %s: %s", certRBACRole.Name, rbacEntity, err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully bound role %s to %s", certRBACRole.Name, rbacEntity)

	return nil
}

const Instructions = `Your users should now be able to setup their local helm client to access Tiller now. To configure their client, they should use the "kubergrunt helm configure" command:

   kubergrunt helm configure --tiller-namespace %s

They must pass in one of --rbac-user, --rbac-group, or --service-account, depending on what entity they are authenticating as.

If they wish to further setup kubectl to default to the managed namespace, they can pass in the following options:

   kubergrunt helm configure \
     --tiller-namespace %s \
     --resource-namespace RESOURCE_NAMESPACE \
     --set-kubectl-namespace
`
