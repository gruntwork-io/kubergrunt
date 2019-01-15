package main

import (
	"crypto/x509/pkix"
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
	"github.com/gruntwork-io/gruntwork-cli/shell"
	"github.com/urfave/cli"

	"github.com/gruntwork-io/kubergrunt/helm"
	"github.com/gruntwork-io/kubergrunt/tls"
)

var (
	// Configurations for how helm is installed
	serviceAccountFlag = cli.StringFlag{
		Name:  "service-account",
		Usage: "The name of the ServiceAccount that Tiller should use.",
	}
	namespaceFlag = cli.StringFlag{
		Name:  "namespace",
		Usage: "Kubernetes namespace to install Tiller in.",
	}

	// Configurations for how to authenticate with the Kubernetes cluster.
	// NOTE: this is the same as eksKubectlContextNameFlag and eksKubeconfigFlag, except the descriptions are updated to
	// fit this series of subcommands.
	helmKubectlContextNameFlag = cli.StringFlag{
		Name:  KubectlContextNameFlagName,
		Usage: "The kubectl config context to use for authenticating with the Kubernetes cluster.",
	}
	helmKubeconfigFlag = cli.StringFlag{
		Name:  KubeconfigFlagName,
		Usage: "The path to the kubectl config file to use to authenticate with Kubernetes. Defaults to ~/.kube/config",
	}

	// Configurations for setting up the TLS certificates
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

	// Configurations for granting and revoking access to clients
	grantedRbacRoleFlag = cli.StringFlag{
		Name:  "rbac-role",
		Usage: "The name of the RBAC role that should be granted access to tiller.",
	}
)

// SetupHelmCommand creates the cli.Command entry for the helm subcommand of kubergrunt
func SetupHelmCommand() cli.Command {
	return cli.Command{
		Name:        "helm",
		Usage:       "Helper commands to configure Helm.",
		Description: "Helper commands to configure Helm, including manging TLS certificates and setting up operator machines to authenticate with Tiller.",
		Subcommands: cli.Commands{
			cli.Command{
				Name:  "deploy",
				Usage: "Install and setup a best practice Helm Server.",
				Description: `Install and setup a best practice Helm Server. In addition to providing a basic Helm Server, this will:

  - Provision TLS certs for the new Helm Server.
  - Setup an RBAC role restricted to the specified namespace and bind it to the specified ServiceAccount.
  - Default to use Secrets for storing Helm Server releases (as opposed to ConfigMaps).
  - Store the private key of the TLS certs in a Secret resource in the kube-system namespace.`,
				Action: deployHelmServer,
				Flags: []cli.Flag{
					serviceAccountFlag,
					namespaceFlag,
					tlsCommonNameFlag,
					tlsOrgFlag,
					tlsOrgUnitFlag,
					tlsCityFlag,
					tlsStateFlag,
					tlsCountryFlag,
					tlsValidityFlag,
					tlsAlgorithmFlag,
					tlsECDSACurveFlag,
					tlsRSABitsFlag,
					helmKubectlContextNameFlag,
					helmKubeconfigFlag,
				},
			},
			cli.Command{
				Name:        "grant",
				Usage:       "Grant access to a deployed Helm server.",
				Description: "Grant access to a deployed Helm server to a client by issuing new TLS certificate keypairs that is accessible by the provided RBAC role.",
				Action:      grantHelmAccess,
				Flags: []cli.Flag{
					namespaceFlag,
					grantedRbacRoleFlag,
					helmKubectlContextNameFlag,
					helmKubeconfigFlag,
				},
			},
			cli.Command{
				Name:        "revoke",
				Usage:       "Revoke access to a deployed Helm server.",
				Description: "Revoke access to a deployed Helm server to a client by issuing new TLS certificate keypairs that is accessible by the provided RBAC role.",
				Action:      revokeHelmAccess,
				Flags: []cli.Flag{
					namespaceFlag,
					grantedRbacRoleFlag,
					helmKubectlContextNameFlag,
					helmKubeconfigFlag,
				},
			},
		},
	}
}

func deployHelmServer(cliContext *cli.Context) error {
	// Check if the required commands are installed
	if err := shell.CommandInstalledE("helm"); err != nil {
		return err
	}
	if err := shell.CommandInstalledE("kubergrunt"); err != nil {
		return err
	}

	// Get required info
	serviceAccount, err := entrypoint.StringFlagRequiredE(cliContext, serviceAccountFlag.Name)
	if err != nil {
		return err
	}
	namespace, err := entrypoint.StringFlagRequiredE(cliContext, namespaceFlag.Name)
	if err != nil {
		return err
	}
	distinguishedName, err := tlsDistinguishedNameFlagsAsPkixName(cliContext)
	if err != nil {
		return err
	}
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}

	// Get additional options
	tlsValidityInDays := cliContext.Int(tlsValidityFlag.Name)
	tlsAlgorithm := cliContext.String(tlsAlgorithmFlag.Name)
	tlsECDSACurve := cliContext.String(tlsECDSACurveFlag.Name)
	tlsRSABits := cliContext.Int(tlsRSABitsFlag.Name)

	// Create tls options struct
	tlsValidity := time.Duration(tlsValidityInDays) * 24 * time.Hour
	tlsOptions := tls.TLSOptions{
		DistinguishedName:   distinguishedName,
		ValidityTimeSpan:    tlsValidity,
		PrivateKeyAlgorithm: tlsAlgorithm,
		ECDSACurve:          tlsECDSACurve,
		RSABits:             tlsRSABits,
	}
	if err := tlsOptions.Validate(); err != nil {
		return err
	}

	return helm.Deploy(
		kubectlOptions,
		namespace,
		serviceAccount,
		tlsOptions,
	)
}

func grantHelmAccess(cliContext *cli.Context) error {
	return nil
}

func revokeHelmAccess(cliContext *cli.Context) error {
	return nil
}

// tlsDistinguishedNameFlagsAsPkixName takes the CLI args related to setting up the Distinguished Name identifier of
// the TLS certificate and converts them to the pkix.Name struct.
func tlsDistinguishedNameFlagsAsPkixName(cliContext *cli.Context) (pkix.Name, error) {
	// The CommonName and Org are required for a valid TLS cert
	commonName, err := entrypoint.StringFlagRequiredE(cliContext, tlsCommonNameFlag.Name)
	if err != nil {
		return pkix.Name{}, err
	}
	org, err := entrypoint.StringFlagRequiredE(cliContext, tlsOrgFlag.Name)
	if err != nil {
		return pkix.Name{}, err
	}

	// The other fields are optional
	orgUnit := cliContext.String(tlsOrgUnitFlag.Name)
	city := cliContext.String(tlsCityFlag.Name)
	state := cliContext.String(tlsStateFlag.Name)
	country := cliContext.String(tlsCountryFlag.Name)

	distinguishedName := pkix.Name{
		CommonName:   commonName,
		Organization: []string{org},
	}
	if orgUnit != "" {
		distinguishedName.OrganizationalUnit = []string{orgUnit}
	}
	if city != "" {
		distinguishedName.Locality = []string{city}
	}
	if state != "" {
		distinguishedName.Province = []string{state}
	}
	if country != "" {
		distinguishedName.Country = []string{country}
	}
	return distinguishedName, nil
}
