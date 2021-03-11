package tls

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
)

const (
	// Private key algorithms
	ECDSAAlgorithm = "ECDSA"
	RSAAlgorithm   = "RSA"

	// Elliptic curves
	P224Curve = "P224"
	P256Curve = "P256"
	P384Curve = "P384"
	P521Curve = "P521"

	// We force users to use at least 2048 bits for RSA, as anything less is cryptographically insecure (since they have
	// been cracked).
	// See https://en.wikipedia.org/wiki/Key_size for more commentary
	MinimumRSABits = 2048
)

var (
	// Valid private key algorithms we support in this library
	PrivateKeyAlgorithms = []string{
		ECDSAAlgorithm,
		RSAAlgorithm,
	}

	// List of known curves we support for ECDSA private key algorithm
	KnownCurves = []string{
		P224Curve,
		P256Curve,
		P384Curve,
		P521Curve,
	}
)

// TLSOptions is a convenient struct to capture all the options needed for generating a TLS certificate key pair.
type TLSOptions struct {
	DistinguishedName   pkix.Name
	ValidityTimeSpan    time.Duration
	PrivateKeyAlgorithm string
	RSABits             int
	ECDSACurve          string
}

// Validate will validate the provided TLSOptions struct is valid.
func (options *TLSOptions) Validate() error {
	switch options.PrivateKeyAlgorithm {
	case ECDSAAlgorithm:
		if !collections.ListContainsElement(KnownCurves, options.ECDSACurve) {
			return errors.WithStackTrace(UnknownECDSACurveError{options.ECDSACurve})
		}
	case RSAAlgorithm:
		if options.RSABits < MinimumRSABits {
			return errors.WithStackTrace(RSABitsTooLow{options.RSABits})
		}
	default:
		return errors.WithStackTrace(UnknownPrivateKeyAlgorithm{options.PrivateKeyAlgorithm})
	}
	return nil
}

// GenerateAndStoreTLSCertificateKeyPair is a convenience method that will select the right underlying functions to use
// to generate the certificate key pairs and store them to disk at the provided root path. The following files will be
// created:
// - name.crt : The x509 certificate file in PEM format.
// - name.pem : The private key file in PEM format.
// - name.pub : The public key file in PEM format.
func (options *TLSOptions) GenerateAndStoreTLSCertificateKeyPair(
	name string,
	rootPath string,
	keyPassword string,
	isCA bool,
	dnsNames []string,
	signedBy *x509.Certificate,
	signedByKey interface{}, // We don't know what format the signing key is in, so we will accept any type
) (CertificateKeyPairPath, error) {
	var err error
	path := CertificateKeyPairPath{
		CertificatePath: filepath.Join(rootPath, fmt.Sprintf("%s.crt", name)),
		PrivateKeyPath:  filepath.Join(rootPath, fmt.Sprintf("%s.pem", name)),
		PublicKeyPath:   filepath.Join(rootPath, fmt.Sprintf("%s.pub", name)),
	}
	switch options.PrivateKeyAlgorithm {
	case ECDSAAlgorithm:
		err = options.generateECDSATLSCertificateKeyPair(path, keyPassword, isCA, dnsNames, signedBy, signedByKey)
	case RSAAlgorithm:
		err = options.generateRSATLSCertificateKeyPair(path, keyPassword, isCA, dnsNames, signedBy, signedByKey)
	default:
		err = errors.WithStackTrace(UnknownPrivateKeyAlgorithm{options.PrivateKeyAlgorithm})
	}
	return path, err
}

func (options *TLSOptions) generateECDSATLSCertificateKeyPair(
	certificateKeyPairPath CertificateKeyPairPath,
	keyPassword string,
	isCA bool,
	dnsNames []string,
	signedBy *x509.Certificate,
	signedByKey interface{}, // We don't know what format the signing key is in, so we will accept any type
) error {
	keypair, err := CreateECDSACertificateKeyPair(options.ValidityTimeSpan, options.DistinguishedName, signedBy, signedByKey, isCA, dnsNames, options.ECDSACurve)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	cert, err := keypair.Certificate()
	if err != nil {
		return err
	}
	err = StoreCertificate(cert, certificateKeyPairPath.CertificatePath)
	if err != nil {
		return err
	}
	err = StoreECDSAPrivateKey(keypair.PrivateKey, keyPassword, certificateKeyPairPath.PrivateKeyPath)
	if err != nil {
		return err
	}
	return StoreECDSAPublicKey(keypair.PublicKey, certificateKeyPairPath.PublicKeyPath)
}

func (options *TLSOptions) generateRSATLSCertificateKeyPair(
	certificateKeyPairPath CertificateKeyPairPath,
	keyPassword string,
	isCA bool,
	dnsNames []string,
	signedBy *x509.Certificate,
	signedByKey interface{}, // We don't know what format the signing key is in, so we will accept any type
) error {
	keypair, err := CreateRSACertificateKeyPair(options.ValidityTimeSpan, options.DistinguishedName, signedBy, signedByKey, isCA, dnsNames, options.RSABits)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	cert, err := keypair.Certificate()
	if err != nil {
		return err
	}
	err = StoreCertificate(cert, certificateKeyPairPath.CertificatePath)
	if err != nil {
		return err
	}
	err = StoreRSAPrivateKey(keypair.PrivateKey, keyPassword, certificateKeyPairPath.PrivateKeyPath)
	if err != nil {
		return err
	}
	return StoreRSAPublicKey(keypair.PublicKey, certificateKeyPairPath.PublicKeyPath)
}
