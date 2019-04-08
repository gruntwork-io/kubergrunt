package main

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/urfave/cli"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
	"github.com/gruntwork-io/kubergrunt/tls"
)

// List out common flag names

const (
	KubeconfigFlagName         = "kubeconfig"
	KubectlContextNameFlagName = "kubectl-context-name"

	// Alternative to using contexts
	KubectlServerFlagName = "kubectl-server-endpoint"
	KubectlCAFlagName     = "kubectl-certificate-authority"
	KubectlTokenFlagName  = "kubectl-token-name"
)

var (
	tlsCommonNameFlag = cli.StringFlag{
		Name:  "tls-common-name",
		Usage: "(Required) The name that will go into the CN (CommonName) field of the identifier.",
	}
	tlsOrgFlag = cli.StringFlag{
		Name:  "tls-org",
		Usage: "(Required) The name of the company that is generating this cert.",
	}
	tlsOrgUnitFlag = cli.StringFlag{
		Name:  "tls-org-unit",
		Usage: "The name of the unit in --tls-org that is generating this cert.",
	}
	tlsCityFlag = cli.StringFlag{
		Name:  "tls-city",
		Usage: "The city where --tls-org is located.",
	}
	tlsStateFlag = cli.StringFlag{
		Name:  "tls-state",
		Usage: "The state where --tls-org is located.",
	}
	tlsCountryFlag = cli.StringFlag{
		Name:  "tls-country",
		Usage: "The country where --tls-org is located.",
	}
	tlsValidityFlag = cli.IntFlag{
		Name:  "tls-validity",
		Value: 3650,
		Usage: "How long the cert will be valid for, in days.",
	}
	tlsAlgorithmFlag = cli.StringFlag{
		Name:  "tls-private-key-algorithm",
		Value: tls.ECDSAAlgorithm,
		Usage: fmt.Sprintf(
			"The name of the algorithm to use for private keys. Must be one of: %s.",
			strings.Join(tls.PrivateKeyAlgorithms, ", "),
		),
	}
	tlsECDSACurveFlag = cli.StringFlag{
		Name:  "tls-private-key-ecdsa-curve",
		Value: tls.P256Curve,
		Usage: fmt.Sprintf(
			"The name of the elliptic curve to use. Should only be used if --tls-private-key-algorithm is %s. Must be one of %s.",
			tls.ECDSAAlgorithm,
			strings.Join(tls.KnownCurves, ", "),
		),
	}
	tlsRSABitsFlag = cli.IntFlag{
		Name:  "tls-private-key-rsa-bits",
		Value: tls.MinimumRSABits,
		Usage: fmt.Sprintf(
			"The size of the generated RSA key in bits. Should only be used if --tls-private-key-algorithm is %s. Must be at least %d.",
			tls.RSAAlgorithm,
			tls.MinimumRSABits,
		),
	}
)

// parseKubectlOptions extracts kubectl related params from CLI flags
func parseKubectlOptions(cliContext *cli.Context) (*kubectl.KubectlOptions, error) {
	logger := logging.GetProjectLogger()

	// Set defaults for the optional parameters, if unset
	var kubectlCA, kubectlToken string
	kubectlServer := cliContext.String(KubectlServerFlagName)
	if kubectlServer != "" {
		logger.Infof("--%s provided. Checking for --%s and --%s.", KubectlServerFlagName, KubectlCAFlagName, KubectlTokenFlagName)
		kubectlCA, err = entrypoint.StringFlagRequiredE(cliContext, KubectlCAFlagName)
		if err != nil {
			return err
		}
		kubectlToken, err = entrypoint.StringFlagRequiredE(cliContext, KubectlTokenFlagName)
		if err != nil {
			return err
		}
	} else {
		logger.Infof("No server provided. Falling back to kubeconfig and context.")
	}

	kubectlContextName := cliContext.String(KubectlContextNameFlagName)
	if kubectlServer == "" && kubectlContextName == "" {
		logger.Infof("No context name provided. Using default.")
	}
	kubeconfigPath := cliContext.String(KubeconfigFlagName)
	if kubectlServer == "" && kubeconfigPath == "" {
		defaultKubeconfigPath, err := kubectl.KubeConfigPathFromHomeDir()
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		kubeconfigPath = defaultKubeconfigPath
		logger.Infof("No kube config path provided. Using default (%s)", kubeconfigPath)
	}

	kubectlOptions := &kubectl.KubectlOptions{
		ContextName:                   kubectlContextName,
		ConfigPath:                    kubeconfigPath,
		Server:                        kubectlServer,
		Base64PEMCertificateAuthority: kubectlCA,
		BearerToken:                   kubectlToken,
	}
	return kubectlOptions, nil
}
