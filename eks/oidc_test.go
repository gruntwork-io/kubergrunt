package eks

import (
	"crypto/tls"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOIDCConfigURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		issuerURL string
		expected  string
	}{
		{
			"base",
			"https://accounts.google.com",
			"https://accounts.google.com/.well-known/openid-configuration",
		},
		{
			"trailing-slash",
			"https://accounts.google.com/",
			"https://accounts.google.com/.well-known/openid-configuration",
		},
		{
			"include-path",
			"https://accounts.google.com/id/1234",
			"https://accounts.google.com/id/1234/.well-known/openid-configuration",
		},
	}

	for _, testCase := range testCases {
		// Capture range variable to bring in scope within for loop to avoid it changing
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			configURL, err := getOIDCConfigURL(testCase.issuerURL)
			assert.NoError(t, err)
			assert.Equal(t, configURL, testCase.expected)
		})
	}
}

func TestGetJwksURL(t *testing.T) {
	const configURL = "https://accounts.google.com/.well-known/openid-configuration"
	const expected = "https://www.googleapis.com/oauth2/v3/certs"
	jwksURL, err := getJwksURL(configURL)
	assert.NoError(t, err)
	assert.Equal(t, jwksURL, expected)
}

func TestGetThumbprint(t *testing.T) {
	const jwksURL = "https://www.googleapis.com/oauth2/v3/certs"

	thumbprint, err := getThumbprint(jwksURL)
	require.NoError(t, err)

	// Google rotates this chain, so do not pin a thumbprint. Compare against the
	// last certificate presented on a direct TLS dial of the same host.
	parsedURL, err := url.Parse(jwksURL)
	require.NoError(t, err)
	conn, err := tls.Dial("tcp", net.JoinHostPort(parsedURL.Hostname(), "443"), &tls.Config{ServerName: parsedURL.Hostname()})
	require.NoError(t, err)
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	require.NotEmpty(t, certs)
	assert.Equal(t, sha1Hash(certs[len(certs)-1].Raw), thumbprint)
}
