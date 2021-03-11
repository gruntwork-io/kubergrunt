package tls

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
)

// TLSRSACertificateKeyPair represents the certificate key pair generated using the RSA algorithm.
type TLSRSACertificateKeyPair struct {
	CertificateBytes []byte
	PrivateKey       *rsa.PrivateKey
	PublicKey        *rsa.PublicKey
}

// Certificate will return the Certificate struct represented by the raw bytes stored on the key pair struct.
func (certificateKeyPair *TLSRSACertificateKeyPair) Certificate() (*x509.Certificate, error) {
	return x509.ParseCertificate(certificateKeyPair.CertificateBytes)
}

// CreateRSACertificateKeyPair will generate a new certificate key pair using the RSA algorithm. You can
// customize the distinguished name on the certificate, the validity time span, whether or not it is a CA certificate,
// and sign the certificate with a given CA using the available parameters.
// The size of the RSA key in bits is configurable. Choosing at least 2048 bits is recommended.
func CreateRSACertificateKeyPair(
	validityTimeSpan time.Duration,
	distinguishedName pkix.Name,
	signedBy *x509.Certificate,
	signedByKey interface{}, // We don't know what format the signing key is in, so we will accept any type
	isCA bool,
	dnsNames []string,
	rsaBits int,
) (TLSRSACertificateKeyPair, error) {
	privateKey, publicKey, err := CreateRSAKeyPair(rsaBits)
	if err != nil {
		return TLSRSACertificateKeyPair{}, errors.WithStackTrace(err)
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
		return TLSRSACertificateKeyPair{}, err
	}

	certificateKeyPair := TLSRSACertificateKeyPair{
		CertificateBytes: certificateBytes,
		PrivateKey:       privateKey,
		PublicKey:        publicKey,
	}
	return certificateKeyPair, nil
}
