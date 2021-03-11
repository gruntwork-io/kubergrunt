package tls

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terratest/modules/logger"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateECDSAKeyPairErrorsOnUnknownCurve(t *testing.T) {
	t.Parallel()

	_, _, err := CreateECDSAKeyPair("unknown")
	assert.Error(t, err)
	switch errors.Unwrap(err).(type) {
	case UnknownECDSACurveError:
	default:
		logger.Log(t, "Wrong error type for CreateECDSAKeyPair using unknown elliptic curve")
		t.Fail()
	}
}

func TestCreateECDSAKeyPairSupportsAllKnownCurves(t *testing.T) {
	t.Parallel()

	for _, curve := range KnownCurves {
		// Capture range variable because it changes (due to for loop) before executing the test function
		curve := curve
		t.Run(curve, func(t *testing.T) {
			t.Parallel()
			privKey, pubKey, err := CreateECDSAKeyPair(curve)
			assert.NoError(t, err)
			assert.NotNil(t, privKey)
			assert.NotNil(t, pubKey)
		})
	}
}

func TestCreateECDSAKeyPairReturnsCompatibleKeys(t *testing.T) {
	t.Parallel()

	privKey, pubKey, err := CreateECDSAKeyPair("P256")
	assert.NoError(t, err)
	privKeyTmpPath := StoreECDSAKeyToTempFile(t, privKey, "")
	defer os.Remove(privKeyTmpPath)
	pubKeyTmpPath := StoreECDSAPublicKeyToTempFile(t, pubKey)
	defer os.Remove(pubKeyTmpPath)

	// Verify the public key matches the private key by regenerating the public key from the private key and verifying
	// it is the same as what we have.
	keyPubFromPrivCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"pkey", "-pubout", "-inform", "PEM", "-in", privKeyTmpPath, "-outform", "PEM"},
	}
	keyPubFromPriv := shell.RunCommandAndGetOutput(t, keyPubFromPrivCmd)
	pubKeyBytes, err := ioutil.ReadFile(pubKeyTmpPath)
	assert.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(pubKeyBytes)), strings.TrimSpace(keyPubFromPriv))
}

func TestStoreECDSAPrivateKeyStoresInPEMFormat(t *testing.T) {
	t.Parallel()

	privKey, _, err := CreateECDSAKeyPair("P256")
	require.NoError(t, err)
	tmpPath := StoreECDSAKeyToTempFile(t, privKey, "")
	defer os.Remove(tmpPath)

	// Verify the format, and that key is unencrypted. We use openssl binary to read in the file and if it doesn't
	// error, then we know the key is formatted correctly.
	// See: https://stackoverflow.com/questions/26259432/how-to-check-a-public-rsa-key-file/26260514#26260514
	cmd := shell.Command{
		Command: "openssl",
		Args:    []string{"ec", "-inform", "PEM", "-in", tmpPath, "-noout"},
	}
	shell.RunCommand(t, cmd)
}

func TestStoreECDSAPrivateKeyEncryption(t *testing.T) {
	t.Parallel()

	uniqueId := random.UniqueId()
	privKey, _, err := CreateECDSAKeyPair("P256")
	require.NoError(t, err)
	tmpPath := StoreECDSAKeyToTempFile(t, privKey, uniqueId)
	defer os.Remove(tmpPath)

	// Verify the format, and that key is encrypted. We use openssl binary to read in the file and if it doesn't
	// error, then we know the key is formatted correctly.
	// See: https://stackoverflow.com/questions/26259432/how-to-check-a-public-rsa-key-file/26260514#26260514
	cmd := shell.Command{
		Command: "openssl",
		Args:    []string{"ec", "-inform", "PEM", "-in", tmpPath, "-passin", fmt.Sprintf("pass:%s", uniqueId), "-noout"},
	}
	shell.RunCommand(t, cmd)
}

// StoreECDSAKeyToTempFile will create a new temporary file and store the provided private key, encrypting it with the
// provided password.
func StoreECDSAKeyToTempFile(t *testing.T, privKey *ecdsa.PrivateKey, password string) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()
	require.NoError(t, StoreECDSAPrivateKey(privKey, password, tmpfile.Name()))
	return tmpfile.Name()
}

// StoreECDSAPublicKeyToTempFile will create a new temporary file and store the provided public key.
func StoreECDSAPublicKeyToTempFile(t *testing.T, pubKey *ecdsa.PublicKey) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()
	require.NoError(t, StoreECDSAPublicKey(pubKey, tmpfile.Name()))
	return tmpfile.Name()
}
