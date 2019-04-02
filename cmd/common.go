package main

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/kubergrunt/tls"
	"github.com/urfave/cli"
)

// List out common flag names

const (
	KubectlContextNameFlagName = "kubectl-context-name"
	KubeconfigFlagName         = "kubeconfig"
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
