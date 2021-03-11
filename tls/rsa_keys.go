package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"

	"github.com/gruntwork-io/go-commons/errors"
)

// StoreRSAPrivateKey takes the given RSA private key, encode it to pem, and store it on disk at the specified path. You
// can optionally provide a password to encrypt the key on disk (passing in "" will store it unencrypted).
func StoreRSAPrivateKey(privateKey *rsa.PrivateKey, password string, path string) error {
	pemBlock, err := EncodeRSAPrivateKeyToPEM(privateKey, password)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return errors.WithStackTrace(StorePEM(pemBlock, path))
}

// StoreRSAPublicKey takes the given RSA public key, encode it to pem, and store it on disk at the specified path. You
func StoreRSAPublicKey(publicKey *rsa.PublicKey, path string) error {
	pemBlock, err := EncodePublicKeyToPEM(publicKey)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return errors.WithStackTrace(StorePEM(pemBlock, path))
}

// CreateRSAKeyPair generates a new private public key pair using the RSA algorithm. The size of the RSA key in bits is
// configurable. We force users to use at least 2048 bits, as anything less is cryptographically insecure (since they
// have been cracked).
// See https://en.wikipedia.org/wiki/Key_size for more commentary
func CreateRSAKeyPair(rsaBits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	if rsaBits < MinimumRSABits {
		err := RSABitsTooLow{rsaBits}
		return nil, nil, errors.WithStackTrace(err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, nil, errors.WithStackTrace(err)
	}
	return privateKey, &privateKey.PublicKey, nil
}

// LoadRSAPrivateKey will load a private key object from the provided path, assuming it holds a certificate encoded in
// PEM.
func LoadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	rawData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	privateKeyPemBlock, _ := pem.Decode(rawData)
	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyPemBlock.Bytes)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return privateKey, nil
}
