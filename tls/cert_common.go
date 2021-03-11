package tls

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"time"

	"github.com/gruntwork-io/go-commons/errors"

	"github.com/gruntwork-io/kubergrunt/kubectl"
)

// CertificateKeyPairPath represents the path where the certificate key pair resides.
type CertificateKeyPairPath struct {
	CertificatePath string
	PrivateKeyPath  string
	PublicKeyPath   string
}

// StoreCertificate will take the provided certificate, encode it to pem, and store it on disk at the specified path.
func StoreCertificate(certificate *x509.Certificate, path string) error {
	pemBlock := EncodeCertificateToPEM(certificate)
	return errors.WithStackTrace(StorePEM(pemBlock, path))
}

// CreateCertificateFromKeys will take the provided key pair and generate the associated TLS certificate. You can
// customize the distinguished name on the certificate, the validity time span, whether or not it is a CA certificate,
// and sign the certificate with a given CA using the available parameters.
// Note: The passed in private key should be the private key of the SIGNER (certificate signing), while the public key
// should be the public key of the SIGNEE (certificate being signed).
// Code based on generate_cert command in crypto/tls: https://golang.org/src/crypto/tls/generate_cert.go
func CreateCertificateFromKeys(
	validityTimeSpan time.Duration,
	distinguishedName pkix.Name,
	signedBy *x509.Certificate,
	isCA bool,
	dnsNames []string,
	pubKey interface{}, // This has to be able to accept the key in any format, like the underlying go func
	privKey interface{}, // This has to be able to accept the key in any format, like the underlying go func
) ([]byte, error) {
	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	template := createCertificateTemplate(serialNumber, distinguishedName, validityTimeSpan, isCA, dnsNames)
	// If signedBy is nil, we will set it to the template so that the generated certificate is self signed
	if signedBy == nil {
		signedBy = &template
	}
	certificateBytes, err := x509.CreateCertificate(rand.Reader, &template, signedBy, pubKey, privKey)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return certificateBytes, nil
}

// generateSerialNumber will generate a random serial number to use for generating a new certificate
func generateSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}

// createCertificateTemplate will generate the Certificate struct with the metadata. The actual certificate data still
// needs to be appended to the struct.
func createCertificateTemplate(
	serialNumber *big.Int,
	distinguishedName pkix.Name,
	validityTimeSpan time.Duration,
	isCA bool,
	dnsNames []string,
) x509.Certificate {
	validFrom := time.Now()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      distinguishedName,

		NotBefore: validFrom,
		NotAfter:  validFrom.Add(validityTimeSpan),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,

		DNSNames: dnsNames,
	}
	if isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	// TODO: make generic so can be used for generating other kinds of certs
	// Add localhost, because the helm client will open a port forwarder via the Kubernetes API to access Tiller.
	// Because of that, helm requires a certificate that allows localhost.
	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"))

	return template
}

// LoadCertificate will load a Certificate object from the provided path, assuming it holds a certificate encoded in PEM.
func LoadCertificate(path string) (*x509.Certificate, error) {
	rawData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	certificatePemBlock, _ := pem.Decode(rawData)
	certificate, err := x509.ParseCertificate(certificatePemBlock.Bytes)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return certificate, nil
}

// StoreCertificateKeyPairAsKubernetesSecret will store the provided certificate key pair (which is available in the
// local file system) in the Kubernetes cluster as a secret.
func StoreCertificateKeyPairAsKubernetesSecret(
	kubectlOptions *kubectl.KubectlOptions,
	secretName string,
	secretNamespace string,
	labels map[string]string,
	annotations map[string]string,
	nameBase string,
	certificateKeyPairPath CertificateKeyPairPath,
	caCertPath string,
) error {
	secret := kubectl.PrepareSecret(secretNamespace, secretName, labels, annotations)
	err := kubectl.AddToSecretFromFile(secret, fmt.Sprintf("%s.crt", nameBase), certificateKeyPairPath.CertificatePath)
	if err != nil {
		return err
	}
	err = kubectl.AddToSecretFromFile(secret, fmt.Sprintf("%s.pem", nameBase), certificateKeyPairPath.PrivateKeyPath)
	if err != nil {
		return err
	}
	err = kubectl.AddToSecretFromFile(secret, fmt.Sprintf("%s.pub", nameBase), certificateKeyPairPath.PublicKeyPath)
	if err != nil {
		return err
	}

	// If we also want to store the CA certificate that can be used to validate server or client
	if caCertPath != "" {
		err = kubectl.AddToSecretFromFile(secret, "ca.crt", caCertPath)
		if err != nil {
			return err
		}
	}

	return kubectl.CreateSecret(kubectlOptions, secret)
}
