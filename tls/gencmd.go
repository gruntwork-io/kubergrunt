package tls

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

const (
	kubernetesSecretPrivateKeyAlgorithmAnnotationKey = "gruntwork.io/private-key-algorithm"
	kubernetesSecretFileNameBaseAnnotationKey        = "gruntwork.io/filename-base"
	kubernetesSecretSignedByAnnotationKey            = "gruntwork.io/signed-by"
)

type KubernetesSecretOptions struct {
	Name        string
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
}

// GenerateAndStoreAsK8SSecret will generate new TLS certificate key pairs and store them as Kubernetes Secret
// resources.
func GenerateAndStoreAsK8SSecret(
	kubectlOptions *kubectl.KubectlOptions,
	secretOptions KubernetesSecretOptions,
	caSecretOptions KubernetesSecretOptions,
	genCA bool,
	filenameBase string,
	tlsOptions TLSOptions,
	dnsNames []string,
) error {
	logger := logging.GetProjectLogger()
	logger.Info("Generating certificate key pairs")

	// Create a temp path to store the certificates
	logger.Info("Creating a temporary directory as a workspace")
	tlsPath, err := ioutil.TempDir("", "")
	if err != nil {
		logger.Errorf("Error creating temp directory to store certificate key pairs: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Using %s as temp path for storing certificates", tlsPath)
	defer func() {
		logger.Infof("Cleaning up temp workspace %s", tlsPath)
		os.RemoveAll(tlsPath)
	}()

	// Generate the certificate key pair
	var keyPairPath CertificateKeyPairPath
	caCertPath := ""
	if genCA {
		logger.Info("Requested CA key pair.")
		keyPairPath, err = generateCAKeyPair(tlsPath, tlsOptions, filenameBase)
		if err != nil {
			return err
		}
	} else {
		logger.Info("Requested signed TLS key pair.")
		caKeyPairPath, caKeyPairAlgorithm, err := loadCAKeyPair(kubectlOptions, caSecretOptions, tlsPath)
		if err != nil {
			return err
		}
		caCertPath = caKeyPairPath.CertificatePath
		keyPairPath, err = generateSignedTLSKeyPair(tlsPath, tlsOptions, caKeyPairPath, caKeyPairAlgorithm, filenameBase, dnsNames)
		if err != nil {
			return err
		}

		// Record which CA was used to sign the resource
		caSignedByString := fmt.Sprintf("namespace=%s,name=%s", caSecretOptions.Namespace, caSecretOptions.Name)
		secretOptions.Annotations[kubernetesSecretSignedByAnnotationKey] = caSignedByString
	}

	// Finally, store the certificate key pair into Kubernetes
	// Augment annotation to indicate private key algorithm and filename base used to generate the cert
	secretOptions.Annotations[kubernetesSecretPrivateKeyAlgorithmAnnotationKey] = tlsOptions.PrivateKeyAlgorithm
	secretOptions.Annotations[kubernetesSecretFileNameBaseAnnotationKey] = filenameBase
	return StoreCertificateKeyPairAsKubernetesSecret(
		kubectlOptions,
		secretOptions.Name,
		secretOptions.Namespace,
		secretOptions.Labels,
		secretOptions.Annotations,
		filenameBase,
		keyPairPath,
		caCertPath,
	)
}

// generateCAKeyPair will issue a new CA TLS certificate key pair.
func generateCAKeyPair(tlsPath string, tlsOptions TLSOptions, filenameBase string) (CertificateKeyPairPath, error) {
	logger := logging.GetProjectLogger()
	logger.Info("Generating CA certificate key pairs and storing into temporary workspace")

	caKeyPairPath, err := tlsOptions.GenerateAndStoreTLSCertificateKeyPair(
		filenameBase,
		tlsPath,
		"", // TODO: support passworded key pairs
		true,
		nil,
		nil,
		nil,
	)
	if err == nil {
		logger.Info("Successfully generated CA TLS certificate key pair and stored in temp workspace.")
	} else {
		logger.Errorf("Error generating CA TLS certificate key pair: %s", err)
	}
	return caKeyPairPath, err
}

// generateSignedTLSKeyPair will issue a new TLS certificate key pair, signed by the provided CA certificate key pair.
func generateSignedTLSKeyPair(
	tlsPath string,
	tlsOptions TLSOptions,
	caKeyPairPath CertificateKeyPairPath,
	caKeyPairAlgorithm string,
	filenameBase string,
	dnsNames []string,
) (CertificateKeyPairPath, error) {
	logger := logging.GetProjectLogger()
	logger.Info("Generating signed certificate key pairs from loaded CA and storing into temporary workspace")

	logger.Info("Parsing CA key pair")
	signingCertificate, err := LoadCertificate(caKeyPairPath.CertificatePath)
	if err != nil {
		logger.Errorf("Error parsing CA TLS certificate key pair: %s", err)
		return CertificateKeyPairPath{}, err
	}
	var signingKey interface{}
	switch caKeyPairAlgorithm {
	case ECDSAAlgorithm:
		signingKey, err = LoadECDSAPrivateKey(caKeyPairPath.PrivateKeyPath)
	case RSAAlgorithm:
		signingKey, err = LoadRSAPrivateKey(caKeyPairPath.PrivateKeyPath)
	default:
		logger.Errorf("Unknown CA key pair algorithm: %s", caKeyPairAlgorithm)
		return CertificateKeyPairPath{}, errors.WithStackTrace(UnknownPrivateKeyAlgorithm{Algorithm: caKeyPairAlgorithm})
	}
	logger.Info("Successfully parsed CA key pair")

	logger.Info("Generating new TLS certificate key pair, signed by CA key pair")
	keyPairPath, err := tlsOptions.GenerateAndStoreTLSCertificateKeyPair(
		filenameBase,
		tlsPath,
		"", // TODO: support passworded key pairs
		false,
		dnsNames,
		signingCertificate,
		signingKey,
	)
	if err == nil {
		logger.Info("Successfully generated TLS certificate key pair and stored in temp workspace.")
	} else {
		logger.Errorf("Error generating TLS certificate key pair: %s", err)
	}
	return keyPairPath, err
}

// loadCAKeyPair loads the CA TLS certificate key pair from a Kubernetes Secret resource. This assumes the CA key pair
// was created using the `tls gen` command of kubergrunt, which imposes a structure to how the TLS certificate key pairs
// are stored in the Secret resource.
func loadCAKeyPair(kubectlOptions *kubectl.KubectlOptions, caSecretOptions KubernetesSecretOptions, tlsPath string) (CertificateKeyPairPath, string, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Loading CA key pair stored in kubernetes secret %s (namespace %s)", caSecretOptions.Name, caSecretOptions.Namespace)

	secret, err := kubectl.GetSecret(kubectlOptions, caSecretOptions.Namespace, caSecretOptions.Name)
	if err != nil {
		logger.Errorf("Error reading Secret resource from Kubernetes: %s", err)
		return CertificateKeyPairPath{}, "", err
	}

	logger.Info("Successfully read Secret resource from Kubernetes.")

	// Now store the certificate key pairs on disk into a temporary location.
	logger.Info("Loading data as CA key pair and storing in temporary workspace")
	filenameBase := secret.Annotations[kubernetesSecretFileNameBaseAnnotationKey]
	certPath := filepath.Join(tlsPath, "ca.crt")
	if err := ioutil.WriteFile(certPath, secret.Data[fmt.Sprintf("%s.crt", filenameBase)], 0600); err != nil {
		logger.Errorf("Error storing CA certificate (ca.crt): %s", err)
		return CertificateKeyPairPath{}, "", errors.WithStackTrace(err)
	}
	privKeyPath := filepath.Join(tlsPath, "ca.pem")
	if err := ioutil.WriteFile(privKeyPath, secret.Data[fmt.Sprintf("%s.pem", filenameBase)], 0600); err != nil {
		logger.Errorf("Error storing CA private key (ca.pem): %s", err)
		return CertificateKeyPairPath{}, "", errors.WithStackTrace(err)
	}
	pubKeyPath := filepath.Join(tlsPath, "ca.pub")
	if err := ioutil.WriteFile(pubKeyPath, secret.Data[fmt.Sprintf("%s.pub", filenameBase)], 0600); err != nil {
		logger.Errorf("Error storing CA public key (ca.pub): %s", err)
		return CertificateKeyPairPath{}, "", errors.WithStackTrace(err)
	}
	logger.Info("Successfully loaded data as CA key pair and stored in temporary workspace")

	algorithm := secret.Annotations[kubernetesSecretPrivateKeyAlgorithmAnnotationKey]

	// Finally build and return the struct
	keyPairPath := CertificateKeyPairPath{
		CertificatePath: certPath,
		PrivateKeyPath:  privKeyPath,
		PublicKeyPath:   pubKeyPath,
	}
	return keyPairPath, algorithm, nil
}
