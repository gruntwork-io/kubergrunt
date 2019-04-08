package main

import (
	"crypto/x509/pkix"
	"fmt"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
	"github.com/gruntwork-io/gruntwork-cli/shell"
	"github.com/urfave/cli"

	"github.com/gruntwork-io/kubergrunt/helm"
	"github.com/gruntwork-io/kubergrunt/tls"
)

const (
	DefaultTillerImage   = "gcr.io/kubernetes-helm/tiller"
	DefaultTillerVersion = "v2.11.0"
)

var (
	// Shared configurations
	tillerNamespaceFlag = cli.StringFlag{
		Name:  "tiller-namespace",
		Usage: "(Required) Kubernetes namespace that Tiller will reside in.",
	}
	resourceNamespaceFlag = cli.StringFlag{
		Name:  "resource-namespace",
		Usage: "(Required) Kubernetes namespace where the resources deployed by Tiller reside.",
	}

	// Configurations for how helm is installed
	serviceAccountFlag = cli.StringFlag{
		Name:  "service-account",
		Usage: "(Required) The name of the ServiceAccount that Tiller should use.",
	}
	tillerImageFlag = cli.StringFlag{
		Name:  "tiller-image",
		Value: DefaultTillerImage,
		Usage: "The container image to use when deploying tiller.",
	}
	tillerVersionFlag = cli.StringFlag{
		Name:  "tiller-version",
		Value: DefaultTillerVersion,
		Usage: "The version of the container image to use when deploying tiller.",
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
		Usage: "The path to the kubectl config file to use to authenticate with Kubernetes. (default: \"~/.kube/config\")",
	}
	helmKubectlServerFlag = cli.StringFlag{
		Name:  KubectlServerFlagName,
		Usage: fmt.Sprintf("The Kubernetes server endpoint where the API is located. Overrides the settings in the kubeconfig. Must also set --%s and --%s.", KubectlCAFlagName, KubectlTokenFlagName),
	}
	helmKubectlCAFlag = cli.StringFlag{
		Name:  KubectlCAFlagName,
		Usage: fmt.Sprintf("The base64 encoded certificate authority data in PEM format to use to validate the Kubernetes server. Overrides the settings in the kubeconfig. Must also set --%s and --%s.", KubectlServerFlagName, KubectlTokenFlagName),
	}
	helmKubectlTokenFlag = cli.StringFlag{
		Name:  KubectlTokenFlagName,
		Usage: fmt.Sprintf("The bearer token to use to authenticate to the Kubernetes server API. Overrides the settings in the kubeconfig. Must also set --%s and --%s.", KubectlServerFlagName, KubectlCAFlagName),
	}

	// Configurations for setting up the TLS certificates.
	// NOTE: the args for setting up the CA and server TLS certificates are defined in cmd/common.go
	clientTLSCommonNameFlag = cli.StringFlag{
		Name:  "client-tls-common-name",
		Usage: "(Required) The name that will go into the CN (CommonName) field of the identifier for the client.",
	}
	clientTLSOrgFlag = cli.StringFlag{
		Name:  "client-tls-org",
		Usage: "(Required) The name of the company that is generating this cert for the client.",
	}
	clientTLSOrgUnitFlag = cli.StringFlag{
		Name:  "client-tls-org-unit",
		Usage: "The name of the unit in --client-tls-org that is generating this cert.",
	}
	clientTLSCityFlag = cli.StringFlag{
		Name:  "client-tls-city",
		Usage: "The city where --client-tls-org is located.",
	}
	clientTLSStateFlag = cli.StringFlag{
		Name:  "client-tls-state",
		Usage: "The state where --client-tls-org is located.",
	}
	clientTLSCountryFlag = cli.StringFlag{
		Name:  "client-tls-country",
		Usage: "The country where --client-tls-org is located.",
	}

	// Configurations for granting and revoking access to clients
	grantedRbacGroupsFlag = cli.StringSliceFlag{
		Name:  "rbac-group",
		Usage: "The name of the RBAC group that should be granted access to tiller. Pass in multiple times for multiple groups.",
	}
	grantedRbacUsersFlag = cli.StringSliceFlag{
		Name:  "rbac-user",
		Usage: "The name of the RBAC user that should be granted access to Tiller. Pass in multiple times for multiple users.",
	}
	grantedServiceAccountsFlag = cli.StringSliceFlag{
		Name:  "rbac-service-account",
		Usage: "The name and namespace of the ServiceAccount (encoded as NAMESPACE/NAME) that should be granted access to tiller. Pass in multiple times for multiple accounts.",
	}

	// Configurations for undeploying helm
	forceUndeployFlag = cli.BoolFlag{
		Name:  "force",
		Usage: "Force removal of the Helm server. Note: this will not delete all deployed releases.",
	}
	undeployReleasesFlag = cli.BoolFlag{
		Name:  "undeploy-releases",
		Usage: "Undeploy all releases managed by the target Helm server before undeploying the server.",
	}
	// This is also used in configure
	helmHomeFlag = cli.StringFlag{
		Name:  "helm-home",
		Usage: "Home directory that is configured for accessing deployed Tiller server. (default: \"~/.helm\")",
	}

	// Configurations for configuring the helm client
	setKubectlNamespaceFlag = cli.BoolFlag{
		Name:  "set-kubectl-namespace",
		Usage: "Set the kubectl context default namespace to match the namespace that Tiller deploys resources into.",
	}
	configuringRBACUserFlag = cli.StringFlag{
		Name:  "rbac-user",
		Usage: "Name of RBAC user that configuration of local helm client is for. Only one of --rbac-user, --rbac-group, or --rbac-service-account can be specified.",
	}
	configuringRBACGroupFlag = cli.StringFlag{
		Name:  "rbac-group",
		Usage: "Name of RBAC group that configuration of local helm client is for. Only one of --rbac-user, --rbac-group, or --rbac-service-account can be specified.",
	}
	configuringServiceAccountFlag = cli.StringFlag{
		Name:  "rbac-service-account",
		Usage: "Name of the Service Account that configuration of local helm client is for. Only one of --rbac-user, --rbac-group, or --rbac-service-account can be specified.",
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
  - Store the private key of the TLS certs in a Secret resource in the kube-system namespace.

Additionally, this command will grant access to an RBAC entity and configure the local helm client to use that using one of "--rbac-user", "--rbac-group", "--rbac-service-account" options.`,
				Action: deployHelmServer,
				Flags: []cli.Flag{
					// Required flags
					serviceAccountFlag,
					tillerNamespaceFlag,
					resourceNamespaceFlag,
					tlsCommonNameFlag,
					tlsOrgFlag,
					clientTLSCommonNameFlag,
					clientTLSOrgFlag,
					configuringRBACUserFlag,
					configuringRBACGroupFlag,
					configuringServiceAccountFlag,

					// Optional flags
					tillerImageFlag,
					tillerVersionFlag,
					tlsOrgUnitFlag,
					tlsCityFlag,
					tlsStateFlag,
					tlsCountryFlag,
					tlsValidityFlag,
					tlsAlgorithmFlag,
					tlsECDSACurveFlag,
					tlsRSABitsFlag,
					clientTLSOrgUnitFlag,
					clientTLSCityFlag,
					clientTLSStateFlag,
					clientTLSCountryFlag,
					helmHomeFlag,
					helmKubectlContextNameFlag,
					helmKubeconfigFlag,
					helmKubectlServerFlag,
					helmKubectlCAFlag,
					helmKubectlTokenFlag,
				},
			},
			cli.Command{
				Name:  "undeploy",
				Usage: "Undeploy a deployed Helm server.",
				Description: `Undeploy a deployed Helm server. This will remove all the resources created as part of deploying the Helm server, including all the Secrets that contain the various certificate key pairs for accessing Helm over TLS.

Note: By default, this will not undeploy the Helm server if there are any deployed releases. You can force removal of the server using the --force option, but this will not delete any releases. If you wish to also delete releases, use the relevant commands in the helm client.`,
				Action: undeployHelmServer,
				Flags: []cli.Flag{
					forceUndeployFlag,
					undeployReleasesFlag,
					helmHomeFlag,
					tillerNamespaceFlag,
					helmKubectlContextNameFlag,
					helmKubeconfigFlag,
					helmKubectlServerFlag,
					helmKubectlCAFlag,
					helmKubectlTokenFlag,
				},
			},
			cli.Command{
				Name:  "configure",
				Usage: "Setup local helm client to be able to access Tiller.",
				Description: `Setup local helm client to be able to access the deployed Tiller located at the provided namespace. This assumes that an administrator has granted you access to the Tiller install already. This will:

- Download the client TLS certificate key pair that you have access to.
- Install the TLS certificate key pair in the helm home directory. The helm home directory can be modified with the --helm-home option.
- Install an environment file compatible with your platform that can be sourced to setup variables to configure default parameters for the helm client to access the Tiller install.
- Optionally set the kubectl context default namespace to be the one that Tiller manages. Note that this will update the kubeconfig file.

You must pass in an identifier for your account. This is either the name of the RBAC user (--rbac-user), RBAC group (--rbac-group), or ServiceAccount (--service-account) that you are authenticating as.`,
				Action: configureHelmClient,
				Flags: []cli.Flag{
					helmHomeFlag,
					configuringRBACUserFlag,
					configuringRBACGroupFlag,
					configuringServiceAccountFlag,
					tillerNamespaceFlag,
					resourceNamespaceFlag,
					setKubectlNamespaceFlag,
					helmKubectlContextNameFlag,
					helmKubeconfigFlag,
					helmKubectlServerFlag,
					helmKubectlCAFlag,
					helmKubectlTokenFlag,
				},
			},
			cli.Command{
				Name:        "grant",
				Usage:       "Grant access to a deployed Helm server.",
				Description: "Grant access to a deployed Helm server to a client by issuing new TLS certificate keypairs that is accessible by the provided RBAC group.",
				Action:      grantHelmAccess,
				Flags: []cli.Flag{
					tillerNamespaceFlag,
					grantedRbacGroupsFlag,
					grantedRbacUsersFlag,
					grantedServiceAccountsFlag,
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
					helmKubectlServerFlag,
					helmKubectlCAFlag,
					helmKubectlTokenFlag,
				},
			},
		},
	}
}

// deployHelmServer is the action function for helm deploy command.
func deployHelmServer(cliContext *cli.Context) error {
	// Check if the required commands are installed
	if err := shell.CommandInstalledE("helm"); err != nil {
		return err
	}

	// Get required info
	serviceAccount, err := entrypoint.StringFlagRequiredE(cliContext, serviceAccountFlag.Name)
	if err != nil {
		return err
	}
	tillerNamespace, err := entrypoint.StringFlagRequiredE(cliContext, tillerNamespaceFlag.Name)
	if err != nil {
		return err
	}
	resourceNamespace, err := entrypoint.StringFlagRequiredE(cliContext, resourceNamespaceFlag.Name)
	if err != nil {
		return err
	}
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}
	tlsOptions, err := parseTLSArgs(cliContext, false)
	if err != nil {
		return err
	}
	clientTLSOptions, err := parseTLSArgs(cliContext, true)
	if err != nil {
		return err
	}

	// Get mutexed info (entity name)
	rbacEntity, err := parseConfigurationRBACEntity(cliContext)
	if err != nil {
		return err
	}

	// Get optional info
	helmHome, err := parseHelmHomeWithDefault(cliContext)
	if err != nil {
		return err
	}
	tillerImage := cliContext.String(tillerImageFlag.Name)
	tillerVersion := cliContext.String(tillerVersionFlag.Name)
	imageSpec := fmt.Sprintf("%s:%s", tillerImage, tillerVersion)

	return helm.Deploy(
		kubectlOptions,
		tillerNamespace,
		resourceNamespace,
		serviceAccount,
		tlsOptions,
		clientTLSOptions,
		helmHome,
		rbacEntity,
		imageSpec,
	)
}

// undeployHelmServer is the action command for the helm undeploy command.
func undeployHelmServer(cliContext *cli.Context) error {
	// Check if the required commands are installed
	if err := shell.CommandInstalledE("helm"); err != nil {
		return err
	}

	// Get required info
	tillerNamespace, err := entrypoint.StringFlagRequiredE(cliContext, tillerNamespaceFlag.Name)
	if err != nil {
		return err
	}
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}

	// Get optional info
	force := cliContext.Bool(forceUndeployFlag.Name)
	undeployReleases := cliContext.Bool(undeployReleasesFlag.Name)
	helmHome, err := parseHelmHomeWithDefault(cliContext)
	if err != nil {
		return err
	}

	return helm.Undeploy(
		kubectlOptions,
		tillerNamespace,
		helmHome,
		force,
		undeployReleases,
	)
}

// configureHelmClient is the action function for the helm configure command.
func configureHelmClient(cliContext *cli.Context) error {
	// Check if the required commands are installed
	if err := shell.CommandInstalledE("helm"); err != nil {
		return err
	}

	// Get required info
	tillerNamespace, err := entrypoint.StringFlagRequiredE(cliContext, tillerNamespaceFlag.Name)
	if err != nil {
		return err
	}
	resourceNamespace, err := entrypoint.StringFlagRequiredE(cliContext, resourceNamespaceFlag.Name)
	if err != nil {
		return err
	}
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}

	// Get mutexed info (entity name)
	rbacEntity, err := parseConfigurationRBACEntity(cliContext)
	if err != nil {
		return err
	}

	// Get optional info
	setKubectlNamespace := cliContext.Bool(setKubectlNamespaceFlag.Name)
	helmHome, err := parseHelmHomeWithDefault(cliContext)
	if err != nil {
		return err
	}

	return helm.ConfigureClient(
		kubectlOptions,
		helmHome,
		tillerNamespace,
		resourceNamespace,
		setKubectlNamespace,
		rbacEntity,
	)
}

// grantHelmAccess is the action function for the helm grant command.
func grantHelmAccess(cliContext *cli.Context) error {
	tillerNamespace, err := entrypoint.StringFlagRequiredE(cliContext, tillerNamespaceFlag.Name)
	if err != nil {
		return err
	}
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}
	tlsOptions, err := parseTLSArgs(cliContext, false)
	if err != nil {
		return err
	}
	rbacGroups := cliContext.StringSlice(grantedRbacGroupsFlag.Name)
	rbacUsers := cliContext.StringSlice(grantedRbacUsersFlag.Name)
	serviceAccounts := cliContext.StringSlice(grantedServiceAccountsFlag.Name)
	if len(rbacGroups) == 0 && len(rbacUsers) == 0 && len(serviceAccounts) == 0 {
		return entrypoint.NewRequiredArgsError("At least one --rbac-group, --rbac-user, or --rbac-service-account is required")
	}
	return helm.GrantAccess(kubectlOptions, tlsOptions, tillerNamespace, rbacGroups, rbacUsers, serviceAccounts)
}

// parseTLSArgs will take CLI args pertaining to TLS and extract out a TLSOptions struct.
func parseTLSArgs(cliContext *cli.Context, isClient bool) (tls.TLSOptions, error) {
	var distinguishedName pkix.Name
	var err error
	if isClient {
		distinguishedName, err = clientTLSDistinguishedNameFlagsAsPkixName(cliContext)
	} else {
		distinguishedName, err = tlsDistinguishedNameFlagsAsPkixName(cliContext)
	}
	if err != nil {
		return tls.TLSOptions{}, err
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
		return tlsOptions, err
	}
	return tlsOptions, nil
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

// clientTLSDistinguishedNameFlagsAsPkixName takes the CLI args related to setting up the Distinguished Name identifier of
// the client side TLS certificate and converts them to the pkix.Name struct.
func clientTLSDistinguishedNameFlagsAsPkixName(cliContext *cli.Context) (pkix.Name, error) {
	// The CommonName and Org are required for a valid TLS cert
	commonName, err := entrypoint.StringFlagRequiredE(cliContext, clientTLSCommonNameFlag.Name)
	if err != nil {
		return pkix.Name{}, err
	}
	org, err := entrypoint.StringFlagRequiredE(cliContext, clientTLSOrgFlag.Name)
	if err != nil {
		return pkix.Name{}, err
	}

	// The other fields are optional
	orgUnit := cliContext.String(clientTLSOrgUnitFlag.Name)
	city := cliContext.String(clientTLSCityFlag.Name)
	state := cliContext.String(clientTLSStateFlag.Name)
	country := cliContext.String(clientTLSCountryFlag.Name)

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

// parseHelmHomeWithDefault will take the helm home option and return it, or the default ~/.helm.
func parseHelmHomeWithDefault(cliContext *cli.Context) (string, error) {
	helmHome := cliContext.String(helmHomeFlag.Name)
	if helmHome == "" {
		return helm.GetDefaultHelmHome()
	}
	return helmHome, nil
}

// parseConfigurationRBACEntity will take the RBAC entity options and return the configured RBAC entity.
func parseConfigurationRBACEntity(cliContext *cli.Context) (helm.RBACEntity, error) {
	configuringRBACUser := cliContext.String(configuringRBACUserFlag.Name)
	configuringRBACGroup := cliContext.String(configuringRBACGroupFlag.Name)
	configuringServiceAccount := cliContext.String(configuringServiceAccountFlag.Name)
	setEntities := 0
	var rbacEntity helm.RBACEntity
	var err error
	if configuringRBACUser != "" {
		setEntities += 1
		rbacEntity = helm.UserInfo{Name: configuringRBACUser}
	}
	if configuringRBACGroup != "" {
		setEntities += 1
		rbacEntity = helm.GroupInfo{Name: configuringRBACGroup}
	}
	if configuringServiceAccount != "" {
		setEntities += 1
		rbacEntity, err = helm.ExtractServiceAccountInfo(configuringServiceAccount)
		if err != nil {
			return rbacEntity, err
		}
	}
	if setEntities != 1 {
		return rbacEntity, MutuallyExclusiveFlagError{"Exactly one of --rbac-user, --rbac-group, or --rbac-service-account must be set"}
	}
	return rbacEntity, nil
}
