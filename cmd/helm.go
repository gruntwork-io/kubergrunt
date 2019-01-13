package main

import (
	"github.com/urfave/cli"
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
	helmKubectlContextNameFlag = cli.StringFlag{
		Name:  "kubectl-context-name, kube-context",
		Usage: "The kubectl config context to use for authenticating with the Kubernetes cluster.",
	}
	helmKubeconfigFlag = cli.StringFlag{
		Name:  "kubeconfig",
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

	// Configurations for granting and revoking access to clients
	grantedRbacRoleFlag = cli.StringFlag{
		Name:  "rbac-role",
		Usage: "The name of the RBAC role that should is granted access to tiller.",
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
	return nil
}

func grantHelmAccess(cliContext *cli.Context) error {
	return nil
}

func revokeHelmAccess(cliContext *cli.Context) error {
	return nil
}
