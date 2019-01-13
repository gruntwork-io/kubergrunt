package kubectl

// Represents common options necessary to specify for all Kubectl calls
type KubectlOptions struct {
	ContextName string
	ConfigPath  string
}

func NewKubectlOptions(contextName string, configPath string) *KubectlOptions {
	return &KubectlOptions{
		ContextName: contextName,
		ConfigPath:  configPath,
	}
}
