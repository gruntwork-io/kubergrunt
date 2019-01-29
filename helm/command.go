package helm

import (
	"github.com/gruntwork-io/gruntwork-cli/shell"

	"github.com/gruntwork-io/kubergrunt/kubectl"
)

// RunHelm will make a call to helm, setting the config and context to the ones specified in the provided options.
func RunHelm(options *kubectl.KubectlOptions, args ...string) error {
	_, err := RunHelmAndGetOutput(options, args...)
	return err
}

// RunHelmAndGetOutput will make a call to helm, setting the config and context to the ones specified in the provided
// options, and return the output of the command.
func RunHelmAndGetOutput(options *kubectl.KubectlOptions, args ...string) (string, error) {
	shellOptions := shell.NewShellOptions()
	cmdArgs := []string{}
	if options.ContextName != "" {
		cmdArgs = append(cmdArgs, "--kube-context", options.ContextName)
	}
	if options.ConfigPath != "" {
		cmdArgs = append(cmdArgs, "--kubeconfig", options.ConfigPath)
	}
	cmdArgs = append(cmdArgs, args...)
	return shell.RunShellCommandAndGetAndStreamOutput(shellOptions, "helm", cmdArgs...)
}
