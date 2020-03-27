package kubectl

import (
	"encoding/base64"
	"io/ioutil"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// AuthScheme is an enum that indicates how to authenticate to the Kubernetes cluster.
type AuthScheme int

const (
	ConfigBased AuthScheme = iota
	DirectAuth
	EKSClusterBased
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

	// EKS based authentication scheme. Has precedence over direct or config based scheme.
	EKSClusterArn string
}

type serverInfo struct {
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

	scheme := options.AuthScheme()
	switch scheme {
	case DirectAuth:
		err = tempConfigFromDirectAuthInfo(
			logger,
			tmpfile,
			serverInfo{
				Server:                        options.Server,
				Base64PEMCertificateAuthority: options.Base64PEMCertificateAuthority,
				BearerToken:                   options.BearerToken,
			},
		)
	case EKSClusterBased:
		err = tempConfigFromEKSClusterInfo(logger, tmpfile, options.EKSClusterArn)
	default:
		return "", errors.WithStackTrace(AuthSchemeNotSupported{scheme})
	}

	return tmpfile.Name(), err
}

func tempConfigFromDirectAuthInfo(logger *logrus.Entry, tmpfile *os.File, serverInfo serverInfo) error {
	config := &api.Config{}
	err := AddClusterToConfig(
		config,
		"default",
		serverInfo.Server,
		serverInfo.Base64PEMCertificateAuthority,
	)
	if err != nil {
		return err
	}

	logger.Infof("Adding auth info to config")
	authInfo := api.NewAuthInfo()
	authInfo.Token = serverInfo.BearerToken
	config.AuthInfos["default"] = authInfo
	logger.Infof("Done adding auth info to config")

	return AddContextToConfig(
		config,
		"default",
		"default",
		"default",
	)
}

func tempConfigFromEKSClusterInfo(logger *logrus.Entry, tmpfile *os.File, eksClusterArn string) error {
	info, err := getKubeCredentialsFromEKSCluster(eksClusterArn)
	if err != nil {
		return err
	}
	return tempConfigFromDirectAuthInfo(logger, tmpfile, *info)
}

func getKubeCredentialsFromEKSCluster(eksClusterArn string) (*serverInfo, error) {
	cluster, err := eksawshelper.GetClusterByArn(eksClusterArn)
	if err != nil {
		return nil, err
	}

	server := aws.StringValue(cluster.Endpoint)
	b64PEMCA := aws.StringValue(cluster.CertificateAuthority.Data)
	token, _, err := eksawshelper.GetKubernetesTokenForCluster(eksClusterArn)
	if err != nil {
		return nil, err
	}

	info := serverInfo{
		Server:                        server,
		Base64PEMCertificateAuthority: b64PEMCA,
		BearerToken:                   token.Token,
	}
	return &info, nil
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

func (options *KubectlOptions) AuthScheme() AuthScheme {
	if options.EKSClusterArn != "" {
		return EKSClusterBased
	} else if options.Server != "" {
		return DirectAuth
	}
	return ConfigBased
}

func authSchemeToString(scheme AuthScheme) string {
	switch scheme {
	case ConfigBased:
		return "config-based"
	case DirectAuth:
		return "direct"
	case EKSClusterBased:
		return "eks-cluster-based"
	}
	// This should not happen
	return "unspecified"
}
