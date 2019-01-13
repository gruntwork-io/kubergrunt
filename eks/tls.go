package eks

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	"net/http"
)

// loadHttpCA takes base64 encoded certificate authority data and and loads a certificate pool that includes the
// provided CA data.
func loadHttpCA(b64CAData string) (*x509.CertPool, error) {
	caCert, err := base64.StdEncoding.DecodeString(b64CAData)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	return caCertPool, nil
}

// loadHttpClientWithCA takes base64 enconded certificate authority data and loads it into an HTTP client that can
// verify TLS endpoints with the CA data.
func loadHttpClientWithCA(b64CAData string) (*http.Client, error) {
	caCertPool, err := loadHttpCA(b64CAData)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}
	return client, nil
}
