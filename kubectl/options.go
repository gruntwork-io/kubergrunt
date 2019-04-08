package kubectl

// Represents common options necessary to specify for all Kubectl calls
type KubectlOptions struct {
	// Config based authentication scheme
	ContextName string
	ConfigPath  string

	// Direct authentication scheme. Has precedence over config based scheme. All 3 values must be set.
	Server                        string
	Base64PEMCertificateAuthority string
	BearerToken                   string
}
