package helm

import (
	"path/filepath"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"k8s.io/helm/pkg/helm"
	"k8s.io/helm/pkg/tlsutil"
)

// NewHelmClient constructs a new helm client that can be used to interact with Tiller.
func NewHelmClient(
	tillerHost string,
	connectionTimeout int64,
	helmHome string,
) (helm.Interface, error) {
	options := []helm.Option{
		helm.Host(tillerHost),
		helm.ConnectTimeout(connectionTimeout),
	}

	tlsopts := tlsutil.Options{
		ServerName:         "",
		KeyFile:            filepath.Join(helmHome, "key.pem"),
		CertFile:           filepath.Join(helmHome, "cert.pem"),
		CaCertFile:         filepath.Join(helmHome, "ca.pem"),
		InsecureSkipVerify: false,
	}
	tlscfg, err := tlsutil.ClientConfig(tlsopts)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	options = append(options, helm.WithTLS(tlscfg))
	client := helm.NewClient(options...)
	return client, nil
}
