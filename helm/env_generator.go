package helm

import (
	"os"
	"path/filepath"
	"text/template"

	"github.com/gruntwork-io/gruntwork-cli/errors"
)

type DeployedHelmInfo struct {
	HelmHome        string
	TillerNamespace string
}

// Render renders a platform specific environment file that can be dot sourced to setup the shell to be able to
// authenticate helm correctly to the deployed Tiller.
// See `env_generator_unix.go` for the unix based env file, and `env_generator_windows.go` for the windows Powershell
// based env file.
func (info DeployedHelmInfo) Render() error {
	envFilePath := filepath.Join(info.HelmHome, envFileName)
	loadedTemplate := template.Must(template.New("helmEnvFile").Parse(envTemplate))
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	file, err := os.OpenFile(envFilePath, flags, 0700)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	err = loadedTemplate.Execute(file, info)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}
