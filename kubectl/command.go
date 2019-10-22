package kubectl

import (
	"io/ioutil"
	"os"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/gruntwork-cli/shell"
)

// RunKubectl will make a call to kubectl, setting the config and context to the ones specified in the provided options.
func RunKubectl(options *KubectlOptions, args ...string) error {
	shellOptions := shell.NewShellOptions()
	cmdArgs := []string{}
	if options.Server != "" {
		caFile, err := options.TempCAFile()
		if err != nil {
			return err
		}
		// Using direct auth, which contains secrets in the CLI args so mark as sensitive
		shellOptions.SensitiveArgs = true
		cmdArgs = append(cmdArgs, "--server", options.Server)
		cmdArgs = append(cmdArgs, "--certificate-authority", caFile)
		cmdArgs = append(cmdArgs, "--token", options.BearerToken)

		// Create a temp file to use as the kubeconfig file. This avoids collision with existing servers that may
		// overlap.
		tmpConfigFile, err := ioutil.TempFile("", "")
		if err != nil {
			return errors.WithStackTrace(err)
		}
		tmpConfigFile.Close()
		defer os.Remove(tmpConfigFile.Name())
		cmdArgs = append(cmdArgs, "--kubeconfig", tmpConfigFile.Name())
	} else {
		if options.ContextName != "" {
			cmdArgs = append(cmdArgs, "--context", options.ContextName)
		}
		if options.ConfigPath != "" {
			cmdArgs = append(cmdArgs, "--kubeconfig", options.ConfigPath)
		}
	}
	cmdArgs = append(cmdArgs, args...)
	_, err := shell.RunShellCommandAndGetAndStreamOutput(shellOptions, "kubectl", cmdArgs...)
	return err
}
