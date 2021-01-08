package helm

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/gruntwork-cli/files"
	homedir "github.com/mitchellh/go-homedir"
	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/helmpath"
	"k8s.io/helm/pkg/repo"

	"github.com/gruntwork-io/kubergrunt/logging"
)

const (
	StableRepositoryName = "stable"
	StableRepositoryURL  = "https://charts.helm.sh/stable"
)

// GetDefaultHelmHome returns the default helm home directory, ~/.helm
func GetDefaultHelmHome() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return home, errors.WithStackTrace(err)
	}
	helmHome := filepath.Join(home, ".helm")
	return helmHome, nil
}

// initializeHelmHome initializes the helm home directory, setting up the necessary folders and repos.
func initializeHelmHome(helmHome string) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Initializing helm home directory %s", helmHome)

	// Ensure the helm home directory exists
	homePath := helmpath.Home(helmHome)
	err := ensureDirectories(homePath)
	if err != nil {
		return err
	}
	logger.Infof("Verified helm home directory %s and all its subdirectories exist", helmHome)

	logger.Infof("Initializing repository file")
	err = ensureDefaultRepos(homePath)
	if err != nil {
		return err
	}
	logger.Infof("Successfully initializing repository file")

	logger.Infof("Successfully initialized helm home directory %s", helmHome)
	return nil
}

// The following is adapted from the helm client, helm/cmd/helm/init.go

// getHomeTree returns the directory tree of the helm home dir that should exist.
func getHomeTree(home helmpath.Home) []string {
	return []string{
		home.String(),
		home.Repository(),
		home.Cache(),
		home.LocalRepository(),
		home.Plugins(),
		home.Starters(),
		home.Archive(),
	}
}

// ensureDirectories makes sure the helm home directory tree is created and exists.
func ensureDirectories(home helmpath.Home) error {
	logger := logging.GetProjectLogger()

	for _, dir := range getHomeTree(home) {
		// Ensure the helm home directory exists
		if !files.FileExists(dir) {
			logger.Infof("Helm home subdirectory %s does not exist. Creating.", dir)
			err := os.MkdirAll(dir, 0700)
			if err != nil {
				logger.Errorf("Error creating helm home subdirectory %s: %s", dir, err)
				return errors.WithStackTrace(err)
			}
		} else if !files.IsDir(dir) {
			return errors.WithStackTrace(HelmHomeIsFileError{dir})
		}
	}
	return nil
}

// ensureDefaultRepos makes sure that the home directory repository file is initialized.
func ensureDefaultRepos(home helmpath.Home) error {
	logger := logging.GetProjectLogger()

	repoFile := home.RepositoryFile()
	if !files.FileExists(repoFile) {
		logger.Infof("Creating helm repository file %s", repoFile)
		newRepoFile := repo.NewRepoFile()

		// Initialize repo file with stable repositories
		logger.Infof("Initializing repository file %s with stable repo", repoFile)
		stableRepoCacheFile := home.CacheIndex(StableRepositoryName)
		stableRepo, err := initStableRepo(stableRepoCacheFile, home)
		if err != nil {
			logger.Errorf("Error initializing repository file %s with stable repo: %s", repoFile, err)
			return err
		}
		newRepoFile.Add(stableRepo)

		// TODO: eventually support more fancy repos, like local, gruntwork, and incubator
		if err := newRepoFile.WriteFile(repoFile, 0644); err != nil {
			logger.Errorf("Error storing repo file %s", repoFile)
			return errors.WithStackTrace(err)
		}
	} else if files.IsDir(repoFile) {
		return errors.WithStackTrace(RepoFileIsDirectoryError{repoFile})
	}
	return ensureRepoFileFormat(repoFile)
}

// initStableRepo initializes the home directory repository file with the stable helm chart repo.
func initStableRepo(cacheFile string, home helmpath.Home) (*repo.Entry, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Adding %s repo with URL: %s", StableRepositoryName, StableRepositoryURL)

	entry := repo.Entry{
		Name:  StableRepositoryName,
		URL:   StableRepositoryURL,
		Cache: cacheFile,
	}
	providers := []getter.Provider{
		getter.Provider{
			Schemes: []string{"http", "https"},
			New: func(URL, CertFile, KeyFile, CAFile string) (getter.Getter, error) {
				return getter.NewHTTPGetter(URL, CertFile, KeyFile, CAFile)
			},
		},
		// TODO: Eventually support helm plugins
	}
	chartRepo, err := repo.NewChartRepository(&entry, providers)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	// In this case, the cacheFile is always absolute. So passing empty string
	// is safe.
	if err := chartRepo.DownloadIndexFile(""); err != nil {
		return nil, RepositoryUnreachableError{RepositoryURL: StableRepositoryURL, UnderlyingError: err}
	}

	return &entry, nil
}

// ensureRepoFileFormat makes sure the repository file is in the right format and is not out of date with the helm
// client version.
func ensureRepoFileFormat(file string) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Verifying repository file format")

	loadedRepo, err := repo.LoadRepositoriesFile(file)
	if err == repo.ErrRepoOutOfDate {
		logger.Infof("Updating repository file format...")
		if err := loadedRepo.WriteFile(file, 0644); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	logger.Infof("Verifed repository file format is up to date")
	return nil
}
