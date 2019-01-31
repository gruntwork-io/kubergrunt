package helm

import (
	"path/filepath"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	homedir "github.com/mitchellh/go-homedir"
)

func GetDefaultHelmHome() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return home, errors.WithStackTrace(err)
	}
	helmHome := filepath.Join(home, ".helm")
	return helmHome, nil
}
