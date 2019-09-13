package eks

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	const expected = "dfe2070c79e7ff36a925ffa327ffe3deecf8f9c2"
	thumbprint, err := getThumbprint(jwksURL)
	assert.NoError(t, err)
	assert.Equal(t, thumbprint, expected)
}
