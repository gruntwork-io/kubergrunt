package tls

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCertificateStoresInPEMFormat(t *testing.T) {
	t.Parallel()

	_, _, certificate, _ := CreateSampleCertKeyPair(t, nil, nil, false)

	tmpPath := StoreCertToTempFile(t, certificate)
	defer os.Remove(tmpPath)

	// Verify the format. We use openssl binary to read in the file and if it doesn't error, then we know the
	// certificate is formatted correctly.
	// See: https://stackoverflow.com/questions/26259432/how-to-check-a-public-rsa-key-file/26260514#26260514
	cmd := shell.Command{
		Command: "openssl",
		Args:    []string{"x509", "-inform", "PEM", "-in", tmpPath, "-noout"},
	}
	shell.RunCommand(t, cmd)
}

func TestCreateCertificateCreatesWithConfiguredMetadata(t *testing.T) {
	t.Parallel()

	_, _, certificate, distinguishedName := CreateSampleCertKeyPair(t, nil, nil, false)

	tmpPath := StoreCertToTempFile(t, certificate)
	defer os.Remove(tmpPath)

	// Verify the certificate information. We use openssl binary to read in the file and if it doesn't error, then we
	// know the certificate is formatted correctly.
	// See: https://stackoverflow.com/questions/26259432/how-to-check-a-public-rsa-key-file/26260514#26260514
	cmd := shell.Command{
		Command: "openssl",
		Args:    []string{"x509", "-inform", "PEM", "-in", tmpPath, "-text", "-noout"},
	}
	out := shell.RunCommandAndGetOutput(t, cmd)

	// openssl text output will encode the distinguished name in the following format
	distinguishedNameString := fmt.Sprintf(
		"C=%s, ST=%s, L=%s, O=%s, OU=%s, CN=%s",
		distinguishedName.Country[0],
		distinguishedName.Province[0],
		distinguishedName.Locality[0],
		distinguishedName.Organization[0],
		distinguishedName.OrganizationalUnit[0],
		distinguishedName.CommonName,
	)
	assert.True(t, strings.Contains(out, distinguishedNameString))

	// Parse out the validity timestamps and verify they are within 5 seconds of expected times
	expectedNotBefore := time.Now()
	expectedNotAfter := expectedNotBefore.Add(1 * time.Hour)
	certNotBefore, certNotAfter := parseValidityTimestampsFromOpensslCertOut(t, out)
	assert.True(t, timeDiffWithin(expectedNotBefore, certNotBefore, 5*time.Second))
	assert.True(t, timeDiffWithin(expectedNotAfter, certNotAfter, 5*time.Second))
}

func TestCreateCertificateCreatesCertificatesCompatibleWithKeys(t *testing.T) {
	t.Parallel()

	privKey, pubKey, certificate, _ := CreateSampleCertKeyPair(t, nil, nil, false)

	certTmpPath := StoreCertToTempFile(t, certificate)
	defer os.Remove(certTmpPath)
	keyTmpPath := StoreRSAKeyToTempFile(t, privKey, "")
	defer os.Remove(keyTmpPath)
	pubKeyTmpPath := StoreRSAPublicKeyToTempFile(t, pubKey)
	defer os.Remove(pubKeyTmpPath)

	// Verify the certificate are for the key pair. This can be done by validating that the modulus is equivalent.
	certModulusCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"x509", "-inform", "PEM", "-in", certTmpPath, "-modulus", "-noout"},
	}
	certModulus := shell.RunCommandAndGetOutput(t, certModulusCmd)
	pubKeyModulusCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"rsa", "-inform", "PEM", "-pubin", "-in", pubKeyTmpPath, "-modulus", "-noout"},
	}
	pubKeyModulus := shell.RunCommandAndGetOutput(t, pubKeyModulusCmd)
	keyModulusCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"rsa", "-inform", "PEM", "-in", keyTmpPath, "-modulus", "-noout"},
	}
	keyModulus := shell.RunCommandAndGetOutput(t, keyModulusCmd)

	assert.Equal(t, certModulus, pubKeyModulus)
	assert.Equal(t, certModulus, keyModulus)
}

func TestCreateCertificateSupportsCreatingCACertsAndSigning(t *testing.T) {
	t.Parallel()

	caPrivKey, _, caCertificate, _ := CreateSampleCertKeyPair(t, nil, nil, true)
	caCertTmpPath := StoreCertToTempFile(t, caCertificate)
	defer os.Remove(caCertTmpPath)
	_, _, signedCertificate, _ := CreateSampleCertKeyPair(t, caCertificate, caPrivKey, false)
	signedCertTmpPath := StoreCertToTempFile(t, signedCertificate)
	defer os.Remove(signedCertTmpPath)

	// Verify the signed certificate is indeed signed by the CA certificate
	verifyCmd := shell.Command{
		Command: "openssl",
		Args:    []string{"verify", "-CAfile", caCertTmpPath, signedCertTmpPath},
	}
	shell.RunCommand(t, verifyCmd)
}

// parseValidityTimestampsFromOpensslCertOut takes the openssl cert text output and looks for the Not Before and Not
// After timestamps, and parses them out as golang Time structs.
func parseValidityTimestampsFromOpensslCertOut(t *testing.T, cmdOut string) (time.Time, time.Time) {
	// This exact time for the layout is significant. DO NOT CHANGE THE TIME!
	// It is used to guide golang where the date parts are in the input string.
	const expectedTimeForm = "Jan  2 15:04:05 2006 GMT"

	beforeRegexp := regexp.MustCompile("Not Before: (.+ GMT)")
	beforeRegexpMatch := beforeRegexp.FindStringSubmatch(cmdOut)
	require.Equal(t, len(beforeRegexpMatch), 2)
	beforeTimestampString := beforeRegexpMatch[1]
	beforeTimestamp, err := time.Parse(expectedTimeForm, beforeTimestampString)
	require.NoError(t, err)

	afterRegexp := regexp.MustCompile("Not After : (.+ GMT)")
	afterRegexpMatch := afterRegexp.FindStringSubmatch(cmdOut)
	require.Equal(t, len(afterRegexpMatch), 2)
	afterTimestampString := afterRegexpMatch[1]
	afterTimestamp, err := time.Parse(expectedTimeForm, afterTimestampString)
	require.NoError(t, err)

	return beforeTimestamp, afterTimestamp
}

// timeDiffWithin checks that the difference in time is within +/- the given duration
func timeDiffWithin(time1 time.Time, time2 time.Time, within time.Duration) bool {
	diff := time1.Sub(time2)
	return -within <= diff && diff <= within
}

// For these tests, we will use RSA keys. See ecdsa_cert_test.go for tests related to ECDSA.
func MustCreateRSAKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	privKey, pubKey, err := CreateRSAKeyPair(2048)
	require.NoError(t, err)
	return privKey, pubKey
}

func CreateSampleDistinguishedName(t *testing.T) pkix.Name {
	return pkix.Name{
		CommonName:         "gruntwork.io",
		Organization:       []string{"Gruntwork"},
		OrganizationalUnit: []string{"IT"},
		Locality:           []string{"Phoenix"},
		Province:           []string{"AZ"},
		Country:            []string{"US"},
	}
}

func CreateSampleCertKeyPair(t *testing.T, signedBy *x509.Certificate, signedByKey interface{}, isCA bool) (*rsa.PrivateKey, *rsa.PublicKey, *x509.Certificate, pkix.Name) {
	privKey, pubKey := MustCreateRSAKeyPair(t)
	distinguishedName := CreateSampleDistinguishedName(t)

	var signingKey interface{}
	signingKey = privKey
	if signedBy != nil {
		signingKey = signedByKey
	}
	certificateBytes, err := CreateCertificateFromKeys(1*time.Hour, distinguishedName, signedBy, isCA, pubKey, signingKey)
	require.NoError(t, err)

	certificate, err := x509.ParseCertificate(certificateBytes)
	require.NoError(t, err)
	return privKey, pubKey, certificate, distinguishedName
}

func StoreCertToTempFile(t *testing.T, cert *x509.Certificate) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()
	require.NoError(t, StoreCertificate(cert, tmpfile.Name()))
	return tmpfile.Name()
}
