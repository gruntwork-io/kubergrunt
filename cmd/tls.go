package main

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
	"github.com/urfave/cli"

	"github.com/gruntwork-io/kubergrunt/tls"
)

var (
	// Required flags
	tlsStoreNamespaceFlag = cli.StringFlag{
		Name:  "namespace",
		Usage: "(Required) Kubernetes namespace that the generated certificates will reside in.",
	}
	tlsSecretNameFlag = cli.StringFlag{
		Name:  "secret-name",
		Usage: "(Required) Name to use for the Kubernetes Secret resource that will store the generated certificates.",
	}

	// CA related flags
	tlsGenCAFlag = cli.BoolFlag{
		Name:  "ca",
		Usage: "When passed in, the generated certificates will be CA key pairs that can be used to issue new signed TLS certificates.",
	}
	tlsCASecretNameFlag = cli.StringFlag{
		Name:  "ca-secret-name",
		Usage: "The name of the Kubernetes Secret resource that holds the CA key pair used to sign the newly generated TLS certificate key pairs. Required when generating signed key pairs.",
	}
	tlsCANamespaceFlag = cli.StringFlag{
		Name:  "ca-namespace",
		Usage: "Kubernetes namespace where the CA key pair is stored in. Defaults to the passed in value for --namespace.",
	}

	// Flags to tag the Kubernetes secret resource
	tlsSecretLabelsFlag = cli.StringSliceFlag{
		Name:  "secret-label",
		Usage: "key=value pair to use to associate a Kubernetes Label with the generated Secret. Pass in multiple times for multiple labels.",
	}
	tlsSecretAnnotationsFlag = cli.StringSliceFlag{
		Name:  "secret-annotation",
		Usage: "key=value pair to use to associate a Kubernetes Annotation with the generated Secret. Pass in multiple times for multiple annotations.",
	}
	tlsSecretFileNameBaseFlag = cli.StringFlag{
		Name:  "secret-filename-base",
		Usage: "Basename to use for the TLS certificate key pair file names when storing in the Kubernetes Secret resource. Defaults to ca when generating CA certs, and tls otherwise.",
	}

	// NOTE: Configurations for setting up the TLS certificates are defined in cmd/common.go

	// Configurations for how to authenticate with the Kubernetes cluster.
	// NOTE: this is the same as eksKubectlContextNameFlag and eksKubeconfigFlag, except the descriptions are updated to
	// fit this series of subcommands.
	tlsKubectlContextNameFlag = cli.StringFlag{
		Name:  KubectlContextNameFlagName,
		Usage: "The name to use for the config context that is set up to authenticate with the Kubernetes cluster.",
	}
	tlsKubeconfigFlag = cli.StringFlag{
		Name:   KubeconfigFlagName,
		Usage:  "The path to the kubectl config file to use to authenticate with Kubernetes. (default: \"~/.kube/config\")",
		EnvVar: "KUBECONFIG",
	}
	tlsKubectlServerFlag = cli.StringFlag{
		Name:  KubectlServerFlagName,
		Usage: fmt.Sprintf("The Kubernetes server endpoint where the API is located. Overrides the settings in the kubeconfig. Must also set --%s and --%s.", KubectlCAFlagName, KubectlTokenFlagName),
	}
	tlsKubectlCAFlag = cli.StringFlag{
		Name:  KubectlCAFlagName,
		Usage: fmt.Sprintf("The base64 encoded certificate authority data in PEM format to use to validate the Kubernetes server. Overrides the settings in the kubeconfig. Must also set --%s and --%s.", KubectlServerFlagName, KubectlTokenFlagName),
	}
	tlsKubectlTokenFlag = cli.StringFlag{
		Name:  KubectlTokenFlagName,
		Usage: fmt.Sprintf("The bearer token to use to authenticate to the Kubernetes server API. Overrides the settings in the kubeconfig. Must also set --%s and --%s.", KubectlServerFlagName, KubectlCAFlagName),
	}
)

func SetupTLSCommand() cli.Command {
	const helpText = "Helper commands to manage TLS certificate key pairs as Kubernetes Secrets."
	return cli.Command{
		Name:        "tls",
		Usage:       helpText,
		Description: helpText,
		Subcommands: cli.Commands{
			cli.Command{
				Name:  "gen",
				Usage: "Generate new certificate key pairs.",
				Description: `Generate new certificate key pairs based on the provided configuration arguments. Once the certificate is generated, it will be stored on your Kubernetes cluster as a Kuberentes Secret resource.

You can generate a CA key pair using the --ca option.

Pass in a --ca-secret-name to sign the newly generated TLS key pair using the CA key pair stored in the Secret with the name provided by --ca-secret-name.`,
				Action: generateTLSCertEntrypoint,
				Flags: []cli.Flag{
					// Secret config flags
					tlsStoreNamespaceFlag,
					tlsSecretNameFlag,
					tlsSecretLabelsFlag,
					tlsSecretAnnotationsFlag,

					// TLS config flags
					tlsGenCAFlag,
					tlsCASecretNameFlag,
					tlsCANamespaceFlag,
					tlsSubjectJsonFlag,
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

					// Kubernetes auth flags
					tlsKubectlContextNameFlag,
					tlsKubeconfigFlag,
					tlsKubectlServerFlag,
					tlsKubectlCAFlag,
					tlsKubectlTokenFlag,
				},
			},
		},
	}
}

// generateTLSCertEntrypoint will parse the CLI args and then call GenerateAndStoreAsK8SSecret.
func generateTLSCertEntrypoint(cliContext *cli.Context) error {
	// Extract required args
	tlsSecretNamespace, err := entrypoint.StringFlagRequiredE(cliContext, tlsStoreNamespaceFlag.Name)
	if err != nil {
		return err
	}
	tlsSecretName, err := entrypoint.StringFlagRequiredE(cliContext, tlsSecretNameFlag.Name)
	if err != nil {
		return err
	}

	// Extract CA options
	genCA := cliContext.Bool(tlsGenCAFlag.Name)
	// caSecretName is required when genCA is false, and ignored when it is true
	caSecretName := ""
	if !genCA {
		caSecretName, err = entrypoint.StringFlagRequiredE(cliContext, tlsCASecretNameFlag.Name)
		if err != nil {
			return err
		}
	}
	// CA Secret Namespace defaults to the same as --namespace
	caSecretNamespace := cliContext.String(tlsCANamespaceFlag.Name)
	if caSecretNamespace == "" {
		caSecretNamespace = tlsSecretNamespace
	}

	// Extract structs based on multiple args
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}
	tlsOptions, err := parseTLSArgs(cliContext, false)
	if err != nil {
		return err
	}

	// Extract optional flags
	tlsSecretLabels := cliContext.StringSlice(tlsSecretLabelsFlag.Name)
	tlsSecretAnnotations := cliContext.StringSlice(tlsSecretAnnotationsFlag.Name)
	tlsSecretFileNameBase := cliContext.String(tlsSecretFileNameBaseFlag.Name)
	if tlsSecretFileNameBase == "" && genCA {
		tlsSecretFileNameBase = "ca"
	} else if tlsSecretFileNameBase == "" && !genCA {
		tlsSecretFileNameBase = "tls"
	}

	// Convert flags to structs
	tlsSecretOptions := tls.KubernetesSecretOptions{
		Name:        tlsSecretName,
		Namespace:   tlsSecretNamespace,
		Labels:      tagArgsToMap(tlsSecretLabels),
		Annotations: tagArgsToMap(tlsSecretAnnotations),
	}
	tlsCASecretOptions := tls.KubernetesSecretOptions{
		Name:        caSecretName,
		Namespace:   caSecretNamespace,
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	return tls.GenerateAndStoreAsK8SSecret(
		kubectlOptions,
		tlsSecretOptions,
		tlsCASecretOptions,
		genCA,
		tlsSecretFileNameBase,
		tlsOptions,
	)
}

// tagArgsToMap takes args used for tags (e.g --secret-label) encoded as a string slice of key=value strings and
// converts to a map.
func tagArgsToMap(tagArgs []string) map[string]string {
	out := map[string]string{}
	for _, tagArg := range tagArgs {
		keyValues := strings.Split(tagArg, "=")
		key := keyValues[0]
		val := strings.Join(keyValues[1:], "=")
		out[key] = val
	}
	return out
}
