package kubectl

import (
	"github.com/gruntwork-io/gruntwork-cli/shell"
)

// RunKubectl will make a call to kubectl, setting the config and context to the ones specified in the provided options.
func RunKubectl(options *KubectlOptions, args ...string) error {
	shellOptions := shell.NewShellOptions()
	cmdArgs := []string{}
	if options.ContextName != "" {
		cmdArgs = append(cmdArgs, "--context", options.ContextName)
	}
	if options.ConfigPath != "" {
		cmdArgs = append(cmdArgs, "--kubeconfig", options.ConfigPath)
	}
	cmdArgs = append(cmdArgs, args...)
	_, err := shell.RunShellCommandAndGetAndStreamOutput(shellOptions, "kubectl", cmdArgs...)
	return err
}
