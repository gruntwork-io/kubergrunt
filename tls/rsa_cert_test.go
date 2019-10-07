package tls

import (
	"os"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/stretchr/testify/require"
)

func TestCreateRSACertificateKeyPairSupportsSigningCerts(t *testing.T) {
	t.Parallel()

	distinguishedName := CreateSampleDistinguishedName(t)
	caKeyPair, err := CreateRSACertificateKeyPair(1*time.Hour, distinguishedName, nil, nil, true, nil, 2048)
	require.NoError(t, err)
	caCert, err := caKeyPair.Certificate()
	require.NoError(t, err)

	signedKeyPair, err := CreateRSACertificateKeyPair(1*time.Hour, distinguishedName, caCert, caKeyPair.PrivateKey, false, nil, 2048)
	require.NoError(t, err)
	signedCert, err := signedKeyPair.Certificate()
	require.NoError(t, err)

	caCertTmpPath := StoreCertToTempFile(t, caCert)
	defer os.Remove(caCertTmpPath)
	signedCertTmpPath := StoreCertToTempFile(t, signedCert)
	defer os.Remove(signedCertTmpPath)

	// Verify the signed certificate is indeed signed by the CA certificate
	verifyCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"verify", "-CAfile", caCertTmpPath, signedCertTmpPath},
	}
	shell.RunCommand(t, verifyCmd)
}

func TestCreateRSACertificateKeyPairSupportsSigningByECDSACerts(t *testing.T) {
	t.Parallel()

	distinguishedName := CreateSampleDistinguishedName(t)
	caKeyPair, err := CreateECDSACertificateKeyPair(1*time.Hour, distinguishedName, nil, nil, true, nil, "P256")
	require.NoError(t, err)
	caCert, err := caKeyPair.Certificate()
	require.NoError(t, err)

	signedKeyPair, err := CreateRSACertificateKeyPair(1*time.Hour, distinguishedName, caCert, caKeyPair.PrivateKey, false, nil, 2048)
	require.NoError(t, err)
	signedCert, err := signedKeyPair.Certificate()
	require.NoError(t, err)

	caCertTmpPath := StoreCertToTempFile(t, caCert)
	defer os.Remove(caCertTmpPath)
	signedCertTmpPath := StoreCertToTempFile(t, signedCert)
	defer os.Remove(signedCertTmpPath)

	// Verify the signed certificate is indeed signed by the CA certificate
	verifyCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"verify", "-CAfile", caCertTmpPath, signedCertTmpPath},
	}
	shell.RunCommand(t, verifyCmd)
}
