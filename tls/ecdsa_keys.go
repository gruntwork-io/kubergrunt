package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"

	"github.com/gruntwork-io/go-commons/errors"
)

// StoreECDSAPrivateKey takes the given ECDSA private key, encode it to pem, and store it on disk at the specified path.
// You can optionally provide a password to encrypt the key on disk (passing in "" will store it unencrypted).
func StoreECDSAPrivateKey(privateKey *ecdsa.PrivateKey, password string, path string) error {
	pemBlock, err := EncodeECDSAPrivateKeyToPEM(privateKey, password)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return errors.WithStackTrace(StorePEM(pemBlock, path))
}

// StoreECDSAPublicKey takes the given ECDSA public key, encode it to pem, and store it on disk at the specified path.
func StoreECDSAPublicKey(publicKey *ecdsa.PublicKey, path string) error {
	pemBlock, err := EncodePublicKeyToPEM(publicKey)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return errors.WithStackTrace(StorePEM(pemBlock, path))
}

// CreateECDSAKeyPair generates a new private public key pair using the ECDSA algorithm. The elliptic curve is
// configurable, and it must be one of P224, P256, P384, P521.
func CreateECDSAKeyPair(ecdsaCurve string) (*ecdsa.PrivateKey, *ecdsa.PublicKey, error) {
	var curve elliptic.Curve
	switch ecdsaCurve {
	case P224Curve:
		curve = elliptic.P224()
	case P256Curve:
		curve = elliptic.P256()
	case P384Curve:
		curve = elliptic.P384()
	case P521Curve:
		curve = elliptic.P521()
	default:
		err := UnknownECDSACurveError{ecdsaCurve}
		return nil, nil, errors.WithStackTrace(err)
	}

	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, errors.WithStackTrace(err)
	}
	return privateKey, &privateKey.PublicKey, nil
}

// LoadECDSAPrivateKey will load a private key object from the provided path, assuming it holds a certificate encoded in
// PEM.
func LoadECDSAPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	rawData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	privateKeyPemBlock, _ := pem.Decode(rawData)
	privateKey, err := x509.ParseECPrivateKey(privateKeyPemBlock.Bytes)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return privateKey, nil
}
