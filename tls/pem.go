package tls

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
)

// EncodeCertificateToPEM will take the raw x509 Certificate and encode it to a pem Block struct.
func EncodeCertificateToPEM(certificate *x509.Certificate) pem.Block {
	return pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificate.Raw,
	}
}

// EncodeRSAPrivateKeyToPEM will take the provided RSA private key and encode it to a pem Block struct. You can
// optionally encrypt the private key by providing a password (passing in "" will keep it unencrypted).
func EncodeRSAPrivateKeyToPEM(privateKey *rsa.PrivateKey, password string) (pem.Block, error) {
	// TODO: make encoding type (PKCS) configurable
	return NewPrivateKeyPEMBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(privateKey), password)
}

// EncodeECDSAPrivateKeyToPEM will take the provided ECDSA private key and encode it to a pem Block struct. You can
// optionally encrypt the private key by providing a password (passing in "" will keep it unencrypted).
func EncodeECDSAPrivateKeyToPEM(privateKey *ecdsa.PrivateKey, password string) (pem.Block, error) {
	blockBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return pem.Block{}, errors.WithStackTrace(err)
	}
	return NewPrivateKeyPEMBlock("EC PRIVATE KEY", blockBytes, password)
}

// EncodePublicKeyToPEM will take the provided public key and encode it to a pem Block struct.
func EncodePublicKeyToPEM(publicKey interface{}) (pem.Block, error) {
	blockBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return pem.Block{}, errors.WithStackTrace(err)
	}
	return pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: blockBytes,
	}, nil
}

// NewPrivateKeyPEMBlock will create the pem Block struct with the provided data. You can optionally encrypt the
// private key by providing a password (passing in "" will keep it unencrypted).
func NewPrivateKeyPEMBlock(pemType string, pemData []byte, password string) (pem.Block, error) {
	block := pem.Block{
		Type:  pemType,
		Bytes: pemData,
	}

	// Encrypt the pem
	if password != "" {
		blockPtr, err := x509.EncryptPEMBlock(rand.Reader, block.Type, block.Bytes, []byte(password), x509.PEMCipherAES256)
		if err != nil {
			return pem.Block{}, errors.WithStackTrace(err)
		}
		block = *blockPtr
	}
	return block, nil
}

// StorePEM will take the pem block and store it to disk.
func StorePEM(pemBlock pem.Block, path string) error {
	var filePermissions os.FileMode
	if strings.HasSuffix(pemBlock.Type, "PRIVATE KEY") || pemBlock.Type == "CERTIFICATE" {
		filePermissions = 0600
	} else {
		filePermissions = 0644
	}

	outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermissions)
	if err != nil {
		return err
	}
	return pem.Encode(outFile, &pemBlock)
}
