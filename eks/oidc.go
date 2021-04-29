package eks

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"

	"github.com/gruntwork-io/go-commons/errors"

	"github.com/gruntwork-io/kubergrunt/logging"
)

type Thumbprint struct {
	Thumbprint string `json:"thumbprint"`
}

type PartialOIDCConfig struct {
	JwksURI string `json:"jwks_uri"`
}

// GetOIDCThumbprint will retrieve the thumbprint of the root CA for the OIDC Provider identified by the issuer URL.
// This is done by first looking up the domain where the keys are provided, and then looking up the TLS certificate
// chain for that domain.
func GetOIDCThumbprint(issuerURL string) (*Thumbprint, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Retrieving OIDC Issuer (%s) CA Thumbprint", issuerURL)

	openidConfigURL, err := getOIDCConfigURL(issuerURL)
	if err != nil {
		logger.Errorf("Error parsing OIDC Issuer URL: %s is not a valid URL", issuerURL)
		return nil, err
	}

	jwksURL, err := getJwksURL(openidConfigURL)
	if err != nil {
		logger.Errorf("Error retrieving JWKS URI from Issuer Config URL %s", openidConfigURL)
		return nil, err
	}

	thumbprint, err := getThumbprint(jwksURL)
	if err != nil {
		logger.Errorf("Error retrieving root CA Thumbprint for JWKS URL %s", jwksURL)
		return nil, err
	}
	logger.Infof("Retrieved OIDC Issuer (%s) CA Thumbprint: %s", issuerURL, thumbprint)
	return &Thumbprint{Thumbprint: thumbprint}, nil
}

// getOIDCConfigURL constructs the URL where you can retrieve the OIDC Config information for a given OIDC provider.
func getOIDCConfigURL(issuerURL string) (string, error) {
	parsedURL, err := url.Parse(issuerURL)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	parsedURL.Path = path.Join(parsedURL.Path, ".well-known", "openid-configuration")
	openidConfigURL := parsedURL.String()
	return openidConfigURL, nil
}

// getJwksURL returns the configured URL where the JWKS keys can be retrieved from the provider.
func getJwksURL(openidConfigURL string) (string, error) {
	resp, err := http.Get(openidConfigURL)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	var partialOIDCConfig PartialOIDCConfig
	if err := json.Unmarshal(body, &partialOIDCConfig); err != nil {
		return "", errors.WithStackTrace(err)
	}

	return partialOIDCConfig.JwksURI, nil
}

// getThumbprint will get the root CA from TLS certificate chain for the FQDN of the JWKS URL.
func getThumbprint(jwksURL string) (string, error) {
	parsedURL, err := url.Parse(jwksURL)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	hostname := parsedURL.Host
	if parsedURL.Port() == "" {
		hostname = net.JoinHostPort(hostname, "443")
	}

	resp, err := http.Get("https://" + hostname)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	peerCerts := resp.TLS.PeerCertificates
	numCerts := len(peerCerts)
	if numCerts == 0 {
		return "", errors.WithStackTrace(NoPeerCertificatesError{jwksURL})
	}

	// root CA certificate is the last one in the list
	root := peerCerts[numCerts-1]
	return sha1Hash(root.Raw), nil
}

// sha1Hash computes the SHA1 of the byte array and returns the hex encoding as a string.
func sha1Hash(data []byte) string {
	hasher := sha1.New()
	hasher.Write(data)
	hashed := hasher.Sum(nil)
	return hex.EncodeToString(hashed)
}
