package tls

import (
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/terratest/modules/logger"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRSAKeyPairErrorsOnTooLowBits(t *testing.T) {
	t.Parallel()

	_, _, err := CreateRSAKeyPair(1024)
	assert.Error(t, err)
	switch errors.Unwrap(err).(type) {
	case RSABitsTooLow:
	default:
		logger.Log(t, "Wrong error type for CreateRSAKeyPair using small key length")
		t.Fail()
	}
}

func TestCreateRSAKeyPairAllows2048KeyLength(t *testing.T) {
	t.Parallel()

	privKey, pubKey, err := CreateRSAKeyPair(2048)
	assert.NoError(t, err)
	assert.NotNil(t, privKey)
	assert.NotNil(t, pubKey)
}

func TestCreateRSAKeyPairReturnsCompatibleKeys(t *testing.T) {
	t.Parallel()

	privKey, pubKey, err := CreateRSAKeyPair(2048)
	assert.NoError(t, err)
	privKeyTmpPath := StoreRSAKeyToTempFile(t, privKey, "")
	defer os.Remove(privKeyTmpPath)
	pubKeyTmpPath := StoreRSAPublicKeyToTempFile(t, pubKey)
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

func TestStoreRSAPrivateKeyStoresInPEMFormat(t *testing.T) {
	t.Parallel()

	privKey, _, err := CreateRSAKeyPair(2048)
	require.NoError(t, err)
	tmpPath := StoreRSAKeyToTempFile(t, privKey, "")
	defer os.Remove(tmpPath)

	// Verify the format, and that key is unencrypted. We use openssl binary to read in the file and if it doesn't
	// error, then we know the key is formatted correctly.
	// See: https://stackoverflow.com/questions/26259432/how-to-check-a-public-rsa-key-file/26260514#26260514
	cmd := shell.Command{
		Command: "openssl",
		Args:    []string{"rsa", "-inform", "PEM", "-in", tmpPath, "-noout"},
	}
	shell.RunCommand(t, cmd)
}

func TestStoreRSAPrivateKeyEncryption(t *testing.T) {
	t.Parallel()

	uniqueId := random.UniqueId()
	privKey, _, err := CreateRSAKeyPair(2048)
	require.NoError(t, err)
	tmpPath := StoreRSAKeyToTempFile(t, privKey, uniqueId)
	defer os.Remove(tmpPath)

	// Verify the format, and that key is encrypted. We use openssl binary to read in the file and if it doesn't
	// error, then we know the key is formatted correctly.
	// See: https://stackoverflow.com/questions/26259432/how-to-check-a-public-rsa-key-file/26260514#26260514
	cmd := shell.Command{
		Command: "openssl",
		Args:    []string{"rsa", "-inform", "PEM", "-in", tmpPath, "-passin", fmt.Sprintf("pass:%s", uniqueId), "-noout"},
	}
	shell.RunCommand(t, cmd)
}

// GetTempFilePath returns a temporary file path that can be used as scratch space.
func GetTempFilePath(t *testing.T) string {
	return WriteStringToTempFile(t, "")
}

// WriteStringToTempFile creates a new temporary file and stores the provided string into it. Returns the path to the
// temporary file.
func WriteStringToTempFile(t *testing.T, data string) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()
	if data != "" {
		_, err := tmpfile.WriteString(data)
		require.NoError(t, err)
	}
	return tmpfile.Name()
}

// StoreRSAKeyToTempFile will create a new temporary file and store the provided private key, encrypting it with the
// provided password.
func StoreRSAKeyToTempFile(t *testing.T, privKey *rsa.PrivateKey, password string) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()
	require.NoError(t, StoreRSAPrivateKey(privKey, password, tmpfile.Name()))
	return tmpfile.Name()
}

// StoreRSAPublicKeyToTempFile will create a new temporary file and store the provided public key.
func StoreRSAPublicKeyToTempFile(t *testing.T, pubKey *rsa.PublicKey) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()
	require.NoError(t, StoreRSAPublicKey(pubKey, tmpfile.Name()))
	return tmpfile.Name()
}
