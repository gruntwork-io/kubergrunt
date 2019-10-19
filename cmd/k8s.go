package main

import (
	"fmt"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
	"github.com/urfave/cli"

	"github.com/gruntwork-io/kubergrunt/kubectl"
)

var (
	ingressNameFlag = cli.StringFlag{
		Name:  "ingress-name",
		Usage: "(Required) The name of the Ingress resource to wait for.",
	}
	namespaceFlag = cli.StringFlag{
		Name:  "namespace",
		Usage: "(Required) The namespace where the Ingress resource to wait for is deployed to.",
	}

	maxRetriesFlag = cli.IntFlag{
		Name:  "max-retries",
		Value: 60,
		Usage: "The maximum number of times to retry checks.",
	}
	sleepBetweenRetriesFlag = cli.DurationFlag{
		Name:  "sleep-between-retries",
		Value: 5 * time.Second,
		Usage: "The amount of time to sleep inbetween each check attempt. Accepted as a duration (5s, 10m, 1h).",
	}

	// Configurations for how to authenticate with the Kubernetes cluster.
	// NOTE: this is the same as helmKubectlContextNameFlag and helmKubeconfigFlag, except the descriptions are updated to
	k8sKubectlContextNameFlag = cli.StringFlag{
		Name:  KubectlContextNameFlagName,
		Usage: "The kubectl config context to use for authenticating with the Kubernetes cluster.",
	}
	k8sKubeconfigFlag = cli.StringFlag{
		Name:   KubeconfigFlagName,
		Usage:  "The path to the kubectl config file to use to authenticate with Kubernetes. You can also set this using the environment variable KUBECONFIG. (default: \"~/.kube/config\")",
		EnvVar: "KUBECONFIG",
	}
	k8sKubectlServerFlag = cli.StringFlag{
		Name:  KubectlServerFlagName,
		Usage: fmt.Sprintf("The Kubernetes server endpoint where the API is located. Use in place of kubeconfig. Must also set --%s and --%s.", KubectlCAFlagName, KubectlTokenFlagName),
	}
	k8sKubectlCAFlag = cli.StringFlag{
		Name:  KubectlCAFlagName,
		Usage: fmt.Sprintf("The base64 encoded certificate authority data in PEM format to use to validate the Kubernetes server. Use in place of kubeconfig. Must also set --%s and --%s.", KubectlServerFlagName, KubectlTokenFlagName),
	}
	k8sKubectlTokenFlag = cli.StringFlag{
		Name:  KubectlTokenFlagName,
		Usage: fmt.Sprintf("The bearer token to use to authenticate to the Kubernetes server API. Use in place of kubeconfig. Must also set --%s and --%s.", KubectlServerFlagName, KubectlCAFlagName),
	}
)

func SetupK8SCommand() cli.Command {
	const helpText = "Helper scripts for managing Kubernetes resources directly."
	return cli.Command{
		Name:        "k8s",
		Usage:       helpText,
		Description: helpText,
		Subcommands: cli.Commands{
			cli.Command{
				Name:  "wait-for-ingress",
				Usage: "Wait for the Ingress endpoint to be provisioned.",
				Description: `Waits for the Ingress endpoint to be provisioned. This will monitor the Ingress resource, continuously checking until the endpoint is allocated to the Ingress resource or times out. By default, this will try for 5 minutes (max retries 60 and time betweeen sleep of 5 seconds).

You can configure the timeout settings using the --max-retries and --sleep-between-retries CLI args. This will check for --max-retries times, sleeping for --sleep-between-retries inbetween tries.`,
				Action: waitForIngressEndpoint,
				Flags: []cli.Flag{
					ingressNameFlag,
					namespaceFlag,

					maxRetriesFlag,
					sleepBetweenRetriesFlag,

					// Kubernetes auth flags
					k8sKubectlContextNameFlag,
					k8sKubeconfigFlag,
					k8sKubectlServerFlag,
					k8sKubectlCAFlag,
					k8sKubectlTokenFlag,
				},
			},
			cli.Command{
				Name:  "kubectl",
				Usage: "Thin wrapper around kubectl to rely on kubergrunt for temporarily authenticating to the cluster.",
				Description: `This command will call out to kubectl with a temporary file that acts as the kubeconfig, set up with the parameters --kubectl-server-endpoint, --kubectl-certificate-authority, --kubectl-token. Unlike using kubectl directly, this command allows you to pass in the base64 encoded certificate authority data directly as opposed to as a file.

To forward args to kubectl, pass all the args you wish to forward after a "--". For example, the following command runs "kubectl get pods -n kube-system":

  kubergrunt k8s kubectl \
    --kubectl-server-endpoint $SERVER_ENDPOINT \
	--kubectl-certificate-authority $SERVER_CA \
	--kubectl-token $TOKEN \
	-- get pods -n kube-system`,
				Action: kubectlWrapper,
				Flags: []cli.Flag{
					// Kubernetes auth flags
					k8sKubectlContextNameFlag,
					k8sKubeconfigFlag,
					k8sKubectlServerFlag,
					k8sKubectlCAFlag,
					k8sKubectlTokenFlag,
				},
			},
		},
	}
}

// waitForIngressEndpoint is the action function for k8s wait-for-ingress command.
func waitForIngressEndpoint(cliContext *cli.Context) error {
	// Extract Kubernetes auth information
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}

	// Retrieve required arguments
	ingressName, err := entrypoint.StringFlagRequiredE(cliContext, ingressNameFlag.Name)
	if err != nil {
		return err
	}
	namespace, err := entrypoint.StringFlagRequiredE(cliContext, namespaceFlag.Name)
	if err != nil {
		return err
	}

	// Retrieve the timeout configuration args
	maxRetries := cliContext.Int(maxRetriesFlag.Name)
	sleepBetweenRetries := cliContext.Duration(sleepBetweenRetriesFlag.Name)

	// Now call waiting logic for the ingress endpoint
	return kubectl.WaitUntilIngressEndpointProvisioned(kubectlOptions, namespace, ingressName, maxRetries, sleepBetweenRetries)
}

// kubectlWrapper is the action function for k8s kubectl command.
func kubectlWrapper(cliContext *cli.Context) error {
	// Extract Kubernetes auth information
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}
	return kubectl.RunKubectl(kubectlOptions, parseKubectlWrapperArgs(cliContext.Args())...)
}

func parseKubectlWrapperArgs(args cli.Args) []string {
	if args.Get(0) == "--" {
		return args[1:]
	}
	return args
}
