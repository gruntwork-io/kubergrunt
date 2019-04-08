package kubectl

import (
	"encoding/base64"
	"io/ioutil"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/gruntwork-io/kubergrunt/logging"
)

// Represents common options necessary to specify for all Kubectl calls
type KubectlOptions struct {
	// Config based authentication scheme
	ContextName string
	ConfigPath  string

	// Direct authentication scheme. Has precedence over config based scheme. All 3 values must be set.
	Server                        string
	Base64PEMCertificateAuthority string
	BearerToken                   string
}

// TempConfigFromAuthInfo will create a temporary kubeconfig file that can be used with commands that don't support
// directly configuring auth info (e.g helm).
func (options *KubectlOptions) TempConfigFromAuthInfo() (string, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Creating temporary file to act as kubeconfig with auth info")

	tmpfile, err := ioutil.TempFile("", "")
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	err = tmpfile.Close()
	if err != nil {
		return tmpfile.Name(), errors.WithStackTrace(err)
	}
	logger.Infof("Created %s to act as temporary kubeconfig file.", tmpfile.Name())

	config := &api.Config{}
	err = AddClusterToConfig(
		config,
		"default",
		options.Server,
		options.Base64PEMCertificateAuthority,
	)
	if err != nil {
		return tmpfile.Name(), err
	}

	logger.Infof("Adding auth info to config")
	authInfo := api.NewAuthInfo()
	authInfo.Token = options.BearerToken
	config.AuthInfos["default"] = authInfo
	logger.Infof("Done adding auth info to config")

	err = AddContextToConfig(
		config,
		"default",
		"default",
		"default",
	)
	return tmpfile.Name(), err
}

// TempCAFile creates a temporary file to hold the Certificate Authority data so that it can be passed on to kubectl.
func (options *KubectlOptions) TempCAFile() (string, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Creating temporary file to hold certificate authority data")

	tmpfile, err := ioutil.TempFile("", "")
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	defer tmpfile.Close()
	logger.Infof("Created %s to hold certificate authority data.", tmpfile.Name())

	caData, err := base64.StdEncoding.DecodeString(options.Base64PEMCertificateAuthority)
	if err != nil {
		return tmpfile.Name(), errors.WithStackTrace(err)
	}
	_, err = tmpfile.Write(caData)
	return tmpfile.Name(), errors.WithStackTrace(err)
}
