package kubectl

import (
	"os"

	"github.com/gruntwork-io/go-commons/shell"
)

// RunKubectl will make a call to kubectl, setting the config and context to the ones specified in the provided options.
func RunKubectl(options *KubectlOptions, args ...string) error {
	shellOptions := shell.NewShellOptions()
	cmdArgs := []string{}
	scheme := options.AuthScheme()
	switch scheme {
	case ConfigBased:
		if options.ContextName != "" {
			cmdArgs = append(cmdArgs, "--context", options.ContextName)
		}
		if options.ConfigPath != "" {
			cmdArgs = append(cmdArgs, "--kubeconfig", options.ConfigPath)
		}
	default:
		tmpfile, err := options.TempConfigFromAuthInfo()
		if tmpfile != "" {
			// Make sure to delete the tmp file at the end
			defer os.Remove(tmpfile)
		}
		if err != nil {
			return err
		}
		cmdArgs = append(cmdArgs, "--kubeconfig", tmpfile)
	}
	cmdArgs = append(cmdArgs, args...)
	_, err := shell.RunShellCommandAndGetAndStreamOutput(shellOptions, "kubectl", cmdArgs...)
	return err
}
