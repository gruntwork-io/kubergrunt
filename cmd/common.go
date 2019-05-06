package main

import (
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
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
	KubectlTokenFlagName  = "kubectl-token"
)

var (
	tlsSubjectJsonFlag = cli.StringFlag{
		Name:  "tls-subject-json",
		Usage: "Provide the TLS subject info as json. You can specify the common name (common_name), org (org), org unit (org_unit), city (city), state (state), and country (country) fields.",
	}
	tlsCommonNameFlag = cli.StringFlag{
		Name:  "tls-common-name",
		Usage: "(Required) The name that will go into the CN (CommonName) field of the identifier. Can be omitted if the information is provided in --tls-subject-json.",
	}
	tlsOrgFlag = cli.StringFlag{
		Name:  "tls-org",
		Usage: "(Required) The name of the company that is generating this cert. Can be omitted if the information is provided in --tls-subject-json.",
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
	tlsSubjectInfoFlags = TLSFlags{
		SubjectInfoJsonFlagName: tlsSubjectJsonFlag.Name,
		CommonNameFlagName:      tlsCommonNameFlag.Name,
		OrgFlagName:             tlsOrgFlag.Name,
		OrgUnitFlagName:         tlsOrgUnitFlag.Name,
		CityFlagName:            tlsCityFlag.Name,
		StateFlagName:           tlsStateFlag.Name,
		CountryFlagName:         tlsCountryFlag.Name,
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

type TLSSubjectInfo struct {
	CommonName string `json:"common_name"`
	Org        string `json:"org" json:"organization"`
	OrgUnit    string `json:"org_unit" json:"organizational_unit"`
	City       string `json:"city" json:"locality"`
	State      string `json:"state" json:"province"`
	Country    string `json:"country"`
}

type TLSFlags struct {
	SubjectInfoJsonFlagName string
	CommonNameFlagName      string
	OrgFlagName             string
	OrgUnitFlagName         string
	CityFlagName            string
	StateFlagName           string
	CountryFlagName         string
}

// parseOrCreateTLSSubjectInfo will parse out the TLS subject json into a TLSSubjectInfo struct. If the string is empty,
// this will create an empty struct that can be filled in based on the CLI args.
func parseOrCreateTLSSubjectInfo(jsonString string) (TLSSubjectInfo, error) {
	var subjectInfo TLSSubjectInfo
	if jsonString != "" {
		err := json.Unmarshal([]byte(jsonString), &subjectInfo)
		if err != nil {
			return subjectInfo, errors.WithStackTrace(err)
		}
	}
	return subjectInfo, nil
}

// parseTLSFlagsToPkixName takes the CLI args related to setting up the Distinguished Name identifier of the TLS
// certificate and converts them to the pkix.Name struct.
func parseTLSFlagsToPkixName(cliContext *cli.Context, tlsFlags TLSFlags) (pkix.Name, error) {
	tlsSubjectInfo, err := parseOrCreateTLSSubjectInfo(cliContext.String(tlsFlags.SubjectInfoJsonFlagName))
	if err != nil {
		return pkix.Name{}, err
	}

	var commonName, org string
	// The CommonName field is required, so it must be provided either in the json or via CLI args
	if tlsSubjectInfo.CommonName == "" {
		commonName, err = entrypoint.StringFlagRequiredE(cliContext, tlsFlags.CommonNameFlagName)
		if err != nil {
			return pkix.Name{}, err
		}
	} else {
		commonName = cliContext.String(tlsFlags.CommonNameFlagName)
	}
	// Override the value if it was provided via CLI
	if commonName != "" {
		tlsSubjectInfo.CommonName = commonName
	}

	// Do the same for org field
	if tlsSubjectInfo.Org == "" {
		org, err = entrypoint.StringFlagRequiredE(cliContext, tlsFlags.OrgFlagName)
		if err != nil {
			return pkix.Name{}, err
		}
	} else {
		org = cliContext.String(tlsFlags.OrgFlagName)
	}
	if org != "" {
		tlsSubjectInfo.Org = org
	}

	// The other fields are optional
	orgUnit := cliContext.String(tlsFlags.OrgUnitFlagName)
	if orgUnit != "" {
		tlsSubjectInfo.OrgUnit = orgUnit
	}
	city := cliContext.String(tlsFlags.CityFlagName)
	if city != "" {
		tlsSubjectInfo.City = city
	}
	state := cliContext.String(tlsFlags.StateFlagName)
	if state != "" {
		tlsSubjectInfo.State = state
	}
	country := cliContext.String(tlsFlags.CountryFlagName)
	if country != "" {
		tlsSubjectInfo.Country = country
	}

	return tlsSubjectInfo.DistinguishedName(), nil
}

// DistinguishedName will return the TLSSubjectInfo as a pkix.Name object.
func (tlsSubjectInfo TLSSubjectInfo) DistinguishedName() pkix.Name {
	distinguishedName := pkix.Name{
		CommonName:   tlsSubjectInfo.CommonName,
		Organization: []string{tlsSubjectInfo.Org},
	}
	if tlsSubjectInfo.OrgUnit != "" {
		distinguishedName.OrganizationalUnit = []string{tlsSubjectInfo.OrgUnit}
	}
	if tlsSubjectInfo.City != "" {
		distinguishedName.Locality = []string{tlsSubjectInfo.City}
	}
	if tlsSubjectInfo.State != "" {
		distinguishedName.Province = []string{tlsSubjectInfo.State}
	}
	if tlsSubjectInfo.Country != "" {
		distinguishedName.Country = []string{tlsSubjectInfo.Country}
	}
	return distinguishedName
}

// parseKubectlOptions extracts kubectl related params from CLI flags
func parseKubectlOptions(cliContext *cli.Context) (*kubectl.KubectlOptions, error) {
	logger := logging.GetProjectLogger()

	// Set defaults for the optional parameters, if unset
	var kubectlCA, kubectlToken string
	var err error
	kubectlServer := cliContext.String(KubectlServerFlagName)
	if kubectlServer != "" {
		logger.Infof("--%s provided. Checking for --%s and --%s.", KubectlServerFlagName, KubectlCAFlagName, KubectlTokenFlagName)
		kubectlCA, err = entrypoint.StringFlagRequiredE(cliContext, KubectlCAFlagName)
		if err != nil {
			return nil, err
		}
		kubectlToken, err = entrypoint.StringFlagRequiredE(cliContext, KubectlTokenFlagName)
		if err != nil {
			return nil, err
		}
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
