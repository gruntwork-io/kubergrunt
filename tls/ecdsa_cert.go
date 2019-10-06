package tls

import (
	"crypto/ecdsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/errors"
)

// TLSECDSACertificateKeyPair represents the certificate key pair generated using the ECDSA algorithm.
type TLSECDSACertificateKeyPair struct {
	CertificateBytes []byte
	PrivateKey       *ecdsa.PrivateKey
	PublicKey        *ecdsa.PublicKey
}

// Certificate will return the Certificate struct represented by the raw bytes stored on the key pair struct.
func (certificateKeyPair *TLSECDSACertificateKeyPair) Certificate() (*x509.Certificate, error) {
	return x509.ParseCertificate(certificateKeyPair.CertificateBytes)
}

// CreateECDSACertificateKeyPair will generate a new certificate key pair using the ECDSA algorithm. You can
// customize the distinguished name on the certificate, the validity time span, whether or not it is a CA certificate,
// and sign the certificate with a given CA using the available parameters.
// The elliptic curve is configurable, and it must be one of P224, P256, P384, P521.
func CreateECDSACertificateKeyPair(
	validityTimeSpan time.Duration,
	distinguishedName pkix.Name,
	signedBy *x509.Certificate,
	signedByKey interface{}, // We don't know what format the signing key is in, so we will accept any type
	isCA bool,
	dnsNames []string,
	ecdsaCurve string,
) (TLSECDSACertificateKeyPair, error) {
	privateKey, publicKey, err := CreateECDSAKeyPair(ecdsaCurve)
	if err != nil {
		return TLSECDSACertificateKeyPair{}, errors.WithStackTrace(err)
	}

	var signingKey interface{}
	signingKey = privateKey
	if signedBy != nil {
		signingKey = signedByKey
	}
	certificateBytes, err := CreateCertificateFromKeys(
		validityTimeSpan,
		distinguishedName,
		signedBy,
		isCA,
		dnsNames,
		publicKey,
		signingKey,
	)
	if err != nil {
		return TLSECDSACertificateKeyPair{}, err
	}

	certificateKeyPair := TLSECDSACertificateKeyPair{
		CertificateBytes: certificateBytes,
		PrivateKey:       privateKey,
		PublicKey:        publicKey,
	}
	return certificateKeyPair, nil
}
