package helm

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/errors"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
	"github.com/gruntwork-io/kubergrunt/tls"
)

const (
	DeployTillerWaitTimeout             = 5 * time.Minute
	DeployTillerWaitSleepBetweenRetries = 1 * time.Second
	TillerDeploymentName                = "tiller-deploy"
)

// Deploy will deploy a new Tiller to the Kubernetes cluster configured with KubectlOptions following best
// practices. Specifically, this will:
// - Require a Namespace and ServiceAccount, so that you will have to explicitly and consciously deploy a super user
//   Tiller to get that.
// - Generate a new set of TLS certs.
// - Store the TLS certs into a Kubernetes Secret into a Namespace that only cluster admins have access to.
// - Deploy Tiller using the generated TLS certs, Namespace, and ServiceAccount. Additionally, set the flags so
//   that the release info is stored in a Secret as opposed to ConfigMap.
// Additionally, if an RBAC entity is passed in, grant access to it and configure the local client at the specified helm
// home directory.
func Deploy(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	resourceNamespace string,
	serviceAccount string,
	tlsOptions tls.TLSOptions,
	clientTLSOptions tls.TLSOptions,
	helmHome string,
	localClientRBACEntity RBACEntity,
	imageSpec string,
) error {
	logger := logging.GetProjectLogger()

	logger.Info("Validating required resources exist.")
	if err := validateRequiredResourcesForDeploy(kubectlOptions, tillerNamespace, serviceAccount); err != nil {
		logger.Error("All required resources do not exist.")
		return err
	}
	logger.Info("All required resources exist.")

	logger.Info("Generating certificate key pairs")
	// Create a temp path to store the certificates
	tlsPath, err := ioutil.TempDir("", "")
	if err != nil {
		logger.Errorf("Error creating temp directory to store certificate key pairs: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Using %s as temp path for storing certificates", tlsPath)
	defer os.RemoveAll(tlsPath)
	caKeyPairPath, tillerKeyPairPath, err := generateCertificateKeyPairs(tlsOptions, tillerNamespace, tlsPath)
	if err != nil {
		logger.Errorf("Error generating certificate key pairs: %s", err)
		return err
	}
	logger.Info("Done generating certificate key pairs")

	// Upload generated CA certs to Kubernetes
	// We will store the CA Certificate Key Pair in the kube-system namespace so that only cluster administrators can
	// access them. The Tiller Certificate Key Pair does not need to be stored separately, as it will be managed by the
	// Tiller Pods when Tiller is deployed.
	logger.Info("Uploading CA certificate key pair as a secret")
	caSecretName := getTillerCACertSecretName(tillerNamespace)
	err = tls.StoreCertificateKeyPairAsKubernetesSecret(
		kubectlOptions,
		caSecretName,
		"kube-system",
		getTillerCACertSecretLabels(tillerNamespace),
		map[string]string{},
		"ca",
		caKeyPairPath,
		"",
	)
	if err != nil {
		logger.Errorf("Error uploading CA certificate key pair as a secret: %s", err)
		return err
	}
	logger.Info("Successfully uploaded CA certificate key pair as a secret")

	// Actually deploy Tiller (helm init call)
	logger.Info("Deploying Helm Server (Tiller)")
	tillerImage, err := InstallTiller(
		kubectlOptions,
		caKeyPairPath,
		tillerKeyPairPath,
		tillerNamespace,
		serviceAccount,
		imageSpec,
	)
	if err != nil {
		logger.Errorf("Error deploying Tiller: %s", err)
		return err
	}
	logger.Infof("Successfully scheduled deployment of Tiller (image: %s) in namespace %s with service account %s", tillerImage, tillerNamespace, serviceAccount)

	logger.Info("Waiting for Tiller to come up")
	err = WaitForTiller(
		kubectlOptions,
		tillerImage,
		tillerNamespace,
		TillerDeploymentName,
		DeployTillerWaitTimeout,
		DeployTillerWaitSleepBetweenRetries,
	)
	logger.Info("Tiller is up and available")

	logger.Info("Granting access and configuring local client")
	err = grantAndConfigureLocalClient(
		kubectlOptions,
		clientTLSOptions,
		tillerNamespace,
		resourceNamespace,
		helmHome,
		localClientRBACEntity,
	)
	if err != nil {
		logger.Infof("Error granting access and configuring local client: %s", err)
		return err
	}
	logger.Info("Successfully granted access and configured local client")

	logger.Info("Done deploying Tiller")
	return nil
}

// validateRequiredResourcesForDeploy ensures the resources required to deploy Helm Server is available on the
// Kubernetes cluster.
func validateRequiredResourcesForDeploy(
	kubectlOptions *kubectl.KubectlOptions,
	namespace string,
	serviceAccount string,
) error {
	logger := logging.GetProjectLogger()

	// Make sure the namespace and service account actually exist
	logger.Infof("Validating the Namespace %s exists", namespace)
	if err := kubectl.ValidateNamespaceExists(kubectlOptions, namespace); err != nil {
		logger.Errorf("Could not find the Namespace %s", namespace)
		return err
	}
	logger.Infof("Found Namespace %s", namespace)
	logger.Infof("Validating the ServiceAccount %s exists in the Namespace %s", serviceAccount, namespace)
	if err := kubectl.ValidateServiceAccountExists(kubectlOptions, namespace, serviceAccount); err != nil {
		logger.Errorf("Could not find the ServiceAccount %s", serviceAccount)
		return err
	}
	logger.Infof("Found ServiceAccount %s", serviceAccount)

	return nil
}

// loadPrivateKeyFromDisk will load a private key encoded as pem from disk. This function does not use a specific type
// for the returned key, because we want to support loading any type of key (ECDSA or RSA).
func loadPrivateKeyFromDisk(tlsOptions tls.TLSOptions, path string) (interface{}, error) {
	switch tlsOptions.PrivateKeyAlgorithm {
	case tls.ECDSAAlgorithm:
		return tls.LoadECDSAPrivateKey(path)
	case tls.RSAAlgorithm:
		return tls.LoadRSAPrivateKey(path)
	default:
		return nil, errors.WithStackTrace(tls.UnknownPrivateKeyAlgorithm{Algorithm: tlsOptions.PrivateKeyAlgorithm})
	}
}

// generateCertificateKeyPair will generate the CA TLS certificate key pair and use that generate another, signed, TLS
// certificate key pair that will be used by the Tiller.
func generateCertificateKeyPairs(tlsOptions tls.TLSOptions, tillerNamespace string, tmpStorePath string) (tls.CertificateKeyPairPath, tls.CertificateKeyPairPath, error) {
	logger := logging.GetProjectLogger()

	logger.Info("Generating CA TLS certificate key pair")
	caKeyPairPath, err := tlsOptions.GenerateAndStoreTLSCertificateKeyPair(
		fmt.Sprintf("tiller_%s_ca", tillerNamespace),
		tmpStorePath,
		"", // TODO: Generate a password
		true,
		nil,
		nil,
		nil,
	)
	if err != nil {
		logger.Errorf("Error generating CA TLS certificate key pair: %s", err)
		return tls.CertificateKeyPairPath{}, tls.CertificateKeyPairPath{}, err
	}
	logger.Info("Done generating CA TLS certificate key pair")

	logger.Info("Generating Tiller TLS certificate key pair (used to identify server)")
	tillerKeyPairPath, err := generateSignedCertificateKeyPair(
		tlsOptions,
		tmpStorePath,
		caKeyPairPath,
		fmt.Sprintf("tiller_%s", tillerNamespace),
	)
	if err != nil {
		logger.Errorf("Error generating Tiller TLS certificate key pair: %s", err)
		return tls.CertificateKeyPairPath{}, tls.CertificateKeyPairPath{}, err
	}
	logger.Info("Successfully generated Tiller TLS certificate key pair (used to identify server)")
	return caKeyPairPath, tillerKeyPairPath, nil
}

func generateSignedCertificateKeyPair(
	tlsOptions tls.TLSOptions,
	tmpStorePath string,
	caKeyPairPath tls.CertificateKeyPairPath,
	nameBase string,
) (tls.CertificateKeyPairPath, error) {
	logger := logging.GetProjectLogger()

	signingCertificate, err := tls.LoadCertificate(caKeyPairPath.CertificatePath)
	if err != nil {
		logger.Errorf("Error loading CA TLS certificate key pair: %s", err)
		return tls.CertificateKeyPairPath{}, err
	}
	signingKey, err := loadPrivateKeyFromDisk(tlsOptions, caKeyPairPath.PrivateKeyPath)
	if err != nil {
		logger.Errorf("Error loading CA TLS certificate key pair: %s", err)
		return tls.CertificateKeyPairPath{}, err
	}

	logger.Info("Generating signed TLS certificate key pair")
	signedKeyPairPath, err := tlsOptions.GenerateAndStoreTLSCertificateKeyPair(
		nameBase,
		tmpStorePath,
		"", // Tiller does not support passwords on the private key
		false,
		nil,
		signingCertificate,
		signingKey,
	)
	if err != nil {
		logger.Errorf("Error generating signed TLS certificate key pair: %s", err)
		return tls.CertificateKeyPairPath{}, err
	}
	logger.Info("Done generating signed TLS Certificate key pair")
	return signedKeyPairPath, nil
}

// grantAndConfigureLocalClient will grant access to the provided RBAC entity and configure the local helm client to use
// that entity's credentials.
func grantAndConfigureLocalClient(
	kubectlOptions *kubectl.KubectlOptions,
	tlsOptions tls.TLSOptions,
	tillerNamespace string,
	resourceNamespace string,
	helmHome string,
	localClientRBACEntity RBACEntity,
) error {
	rbacUsers := []string{}
	rbacGroups := []string{}
	rbacServiceAccounts := []string{}
	switch localClientRBACEntity.EntityType() {
	case "user":
		rbacUsers = append(rbacUsers, localClientRBACEntity.EntityID())
	case "group":
		rbacGroups = append(rbacGroups, localClientRBACEntity.EntityID())
	case "service-account":
		rbacServiceAccounts = append(rbacServiceAccounts, localClientRBACEntity.(ServiceAccountInfo).String())
	default:
		return errors.WithStackTrace(UnknownRBACEntityType{localClientRBACEntity.EntityType()})
	}

	err := GrantAccess(kubectlOptions, tlsOptions, tillerNamespace, rbacGroups, rbacUsers, rbacServiceAccounts)
	if err != nil {
		return err
	}

	return ConfigureClient(kubectlOptions, helmHome, tillerNamespace, resourceNamespace, false, localClientRBACEntity)
}
