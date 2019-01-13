package main

import (
	"fmt"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/gruntwork-cli/shell"
	"github.com/kubernetes-sigs/aws-iam-authenticator/pkg/token"
	"github.com/urfave/cli"

	"github.com/gruntwork-io/package-k8s/modules/kubergrunt/eks"
	"github.com/gruntwork-io/package-k8s/modules/kubergrunt/kubectl"
	"github.com/gruntwork-io/package-k8s/modules/kubergrunt/logging"
)

var (
	eksClusterArnFlag = cli.StringFlag{
		Name:  "eks-cluster-arn",
		Usage: "(Required) The AWS ARN of the EKS cluster that kubectl should authenticate against.",
	}
	waitFlag = cli.BoolFlag{
		Name:  "wait",
		Usage: "Whether or not to wait for the command to succeed.",
	}
	kubectlContextNameFlag = cli.StringFlag{
		Name:  "kubectl-context-name",
		Usage: "The name to use for the config context that is set up to authenticate with the EKS cluster. Defaults to the cluster ARN.",
	}
	kubeconfigFlag = cli.StringFlag{
		Name:  "kubeconfig",
		Usage: "The path to the kubectl config file to setup. Defaults to ~/.kube/config",
	}
	clusterIDFlag = cli.StringFlag{
		Name:  "cluster-id, i",
		Usage: "The name of the EKS cluster for which to retrieve an auth token for.",
	}

	clusterRegionFlag = cli.StringFlag{
		Name:  "region",
		Usage: "(Required) The AWS region code (e.g us-east-1) where the autoscaling group and EKS cluster is located.",
	}
	clusterAsgNameFlag = cli.StringFlag{
		Name:  "asg-name",
		Usage: "(Required) The name of the autoscaling group that is a part of the EKS cluster.",
	}
	drainTimeoutFlag = cli.DurationFlag{
		Name:  "drain-timeout",
		Value: 15 * time.Minute,
		Usage: "The length of time as duration (e.g 10m = 10 minutes) to wait for draining nodes before giving up, zero means infinite. Defaults to 15 minutes.",
	}
	waitMaxRetriesFlag = cli.IntFlag{
		Name:  "max-retries",
		Value: 0,
		Usage: "The maximum number of retries for retry loops during the command. The total amount of time this command will try is based on max-retries and sleep-between-retries. Defaults to heuristic based on 5 minutes per stage of action. Refer to the command documentation for more details.",
	}
	waitSleepBetweenRetriesFlag = cli.DurationFlag{
		Name:  "sleep-between-retries",
		Value: 15 * time.Second,
		Usage: "The amount of time to sleep between retries as duration (e.g 10m = 10 minutes) for retry loops during the command. The total amount of time this command will try is based on max-retries and sleep-between-retries. Defaults to 15 seconds.",
	}
)

// SetupEksCommand creates the cli.Command entry for the eks subcommand of kubergrunt
func SetupEksCommand() cli.Command {
	helpText := "Helper commands to configure EKS, including setting up operator machines to authenticate with EKS."
	return cli.Command{
		Name:        "eks",
		Usage:       helpText,
		Description: helpText,
		Subcommands: cli.Commands{
			cli.Command{
				Name:        "verify",
				Usage:       "Verifies the cluster endpoint for the EKS cluster.",
				Description: "This will verify that the Kubernetes API server is up and accepting traffic for the specified EKS cluster. This does not verify kubectl authentication: use kubectl directly for that purpose.",
				Action:      verifyCluster,
				Flags: []cli.Flag{
					eksClusterArnFlag,
					waitFlag,
					waitMaxRetriesFlag,
					waitSleepBetweenRetriesFlag,
				},
			},
			cli.Command{
				Name:        "configure",
				Usage:       "Set up kubectl to be able to authenticate with EKS.",
				Description: "This will add a new context to the kubectl config that is setup to authenticate with the Kubernetes cluster provided by EKS using aws-iam-authenticator.",
				Action:      setupKubectl,
				Flags: []cli.Flag{
					eksClusterArnFlag,
					kubectlContextNameFlag,
					kubeconfigFlag,
				},
			},
			cli.Command{
				Name:        "token",
				Usage:       "Get token for Kubernetes using AWS IAM credential.",
				Description: "Provides the same functionality as aws-iam-authenticator by integrating with the tool as a library. Provided for convenience to avoid another installation.",
				Action:      getAuthToken,
				Flags: []cli.Flag{
					clusterIDFlag,
				},
			},
			cli.Command{
				Name:  "deploy",
				Usage: "Zero downtime roll out of cluster updates to worker nodes.",
				Description: `Performs a zero downtime rolling deployment of changes to the underlying EC2 instances in an EKS cluster. This subcommand will:

  1. Double the desired capacity of the Auto Scaling Group that powers the EKS Cluster. This will launch new EKS workers with the new launch configuration.
  2. Wait for the new nodes to be ready for Pod scheduling in Kubernetes.
  3. Drain the pods scheduled on the old EKS workers (using the equivalent of "kubectl drain"), so that they will be rescheduled on the new EKS workers.
  4. Wait for all the pods to migrate off of the old EKS workers.
  5. Set the desired capacity down to the original value and remove the old EKS workers from the ASG.

Note that to minimize service disruption from this command, your services should setup a PodDisruptionBudget, a readiness probe that fails on container shutdown events, and implement graceful handling of SIGTERM in the container.

This command includes retry loops for certain stages (e.g waiting for the ASG to scale up). This retry loop is configurable with the options --max-retries and --sleep-between-retries. The command will try up to --max-retries times, sleeping for the duration specified by --sleep-between-retries inbetween each failed attempt.

If max-retries is unspecified, this command will use a value that translates to a total wait time of 5 minutes per wave of ASG, where each wave is 10 instances. For example, if the number of instances in the ASG is 15 instances, this translates to 2 waves, which leads to a total wait time of 10 minutes. To achieve a 10 minute wait time with the default sleep between retries (15 seconds), the max retries needs to be set to 40.
`,
				Action: rollOutDeployment,
				Flags: []cli.Flag{
					clusterRegionFlag,
					clusterAsgNameFlag,
					kubectlContextNameFlag,
					kubeconfigFlag,
					drainTimeoutFlag,
					waitMaxRetriesFlag,
					waitSleepBetweenRetriesFlag,
				},
			},
		},
	}
}

// Command action for `kubergrunt eks verify`
func verifyCluster(cliContext *cli.Context) error {
	// Check for required flags
	eksClusterArn, err := entrypoint.StringFlagRequiredE(cliContext, eksClusterArnFlag.Name)
	if err != nil {
		return err
	}
	wait := cliContext.Bool(waitFlag.Name)
	waitMaxRetries := cliContext.Int(waitMaxRetriesFlag.Name)
	waitSleepBetweenRetries := cliContext.Duration(waitSleepBetweenRetriesFlag.Name)
	return eks.VerifyCluster(eksClusterArn, wait, waitMaxRetries, waitSleepBetweenRetries)
}

// parseKubectlOptions extracts kubectl related params from CLI flags
func parseKubectlOptions(cliContext *cli.Context) (*kubectl.KubectlOptions, error) {
	logger := logging.GetProjectLogger()

	// Set defaults for the optional parameters, if unset
	kubectlContextName := cliContext.String(kubectlContextNameFlag.Name)
	if kubectlContextName == "" {
		logger.Infof("No context name provided. Using default.")
	}
	kubeconfigPath := cliContext.String(kubeconfigFlag.Name)
	if kubeconfigPath == "" {
		defaultKubeconfigPath, err := kubectl.KubeConfigPathFromHomeDir()
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		kubeconfigPath = defaultKubeconfigPath
		logger.Infof("No kube config path provided. Using default (%s)", kubeconfigPath)
	}

	return kubectl.NewKubectlOptions(kubectlContextName, kubeconfigPath), nil
}

// Command action for `kubergrunt eks configure`
func setupKubectl(cliContext *cli.Context) error {
	// Check for required flags
	eksClusterArn, err := entrypoint.StringFlagRequiredE(cliContext, eksClusterArnFlag.Name)
	if err != nil {
		return err
	}

	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}
	// Default context name to cluster ARN
	if kubectlOptions.ContextName == "" {
		kubectlOptions.ContextName = eksClusterArn
	}

	// Check if the required commands are installed
	if err := shell.CommandInstalledE("kubectl"); err != nil {
		return errors.WithStackTrace(err)
	}

	cluster, err := eks.GetClusterByArn(eksClusterArn)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return eks.ConfigureKubectlForEks(
		cluster,
		kubectlOptions,
	)
}

// Command action for `kubergrunt eks token`
func getAuthToken(cliContext *cli.Context) error {
	clusterID, err := entrypoint.StringFlagRequiredE(cliContext, "cluster-id")
	if err != nil {
		return errors.WithStackTrace(err)
	}

	gen, err := token.NewGenerator(false)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	tok, err := gen.Get(clusterID)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	out := gen.FormatJSON(tok)
	// `kubectl` will parse the JSON from stdout to read in what token to use for authenticating with the cluster.
	fmt.Println(out)
	return nil
}

// Command action for `kubergrunt eks deploy`
func rollOutDeployment(cliContext *cli.Context) error {
	kubectlOptions, err := parseKubectlOptions(cliContext)
	if err != nil {
		return err
	}

	region, err := entrypoint.StringFlagRequiredE(cliContext, clusterRegionFlag.Name)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	asgName, err := entrypoint.StringFlagRequiredE(cliContext, clusterAsgNameFlag.Name)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	drainTimeout := cliContext.Duration(drainTimeoutFlag.Name)
	waitMaxRetries := cliContext.Int(waitMaxRetriesFlag.Name)
	waitSleepBetweenRetries := cliContext.Duration(waitSleepBetweenRetriesFlag.Name)

	return eks.RollOutDeployment(
		region,
		asgName,
		kubectlOptions,
		drainTimeout,
		waitMaxRetries,
		waitSleepBetweenRetries,
	)
}
