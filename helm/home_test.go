package helm

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/gruntwork-cli/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/helm/pkg/helm/helmpath"
	"k8s.io/helm/pkg/repo"
)

func TestHelmHomeEnsureDefaultReposRequiresRepoFileToBeFile(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	defer os.RemoveAll(dir)
	require.NoError(t, err)
	dirAsHelmHome := helmpath.Home(dir)
	err = os.MkdirAll(dirAsHelmHome.RepositoryFile(), 0700)
	require.NoError(t, err)

	err = ensureDefaultRepos(dirAsHelmHome)
	assert.Error(t, err)

	// Verify the right error type is returned to make sure it is failing in the expected way
	switch errors.Unwrap(err).(type) {
	case RepoFileIsDirectoryError:
	default:
		t.Fatalf("Error %s is not of type RepoFileIsDirectoryError", err)
	}
}

func TestHelmHomeEnsureDefaultReposCreatesRepoFile(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	defer os.RemoveAll(dir)
	require.NoError(t, err)
	dirAsHelmHome := helmpath.Home(dir)
	require.NoError(t, ensureDirectories(dirAsHelmHome))

	err = ensureDefaultRepos(dirAsHelmHome)
	require.NoError(t, err)
	verifyRepoFile(t, dirAsHelmHome)
}

func TestHelmHomeInitializeHelmHomeRequiresHelmHomePathToNotBeFile(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "")
	file.Close()
	fname := file.Name()
	defer os.Remove(fname)
	require.NoError(t, err)

	err = initializeHelmHome(fname)
	assert.Error(t, err)
	// Verify the right error type is returned to make sure it is failing in the expected way
	switch errors.Unwrap(err).(type) {
	case HelmHomeIsFileError:
	default:
		t.Fatalf("Error %s is not of type HelmHomeIsFileError", err)
	}
}

func TestHelmHomeEnsureDirectoriesCreatesAllSubDirs(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	defer os.RemoveAll(dir)
	require.NoError(t, err)

	fakeHelmHome := filepath.Join(dir, ".helm")
	dirAsHelmHome := helmpath.Home(fakeHelmHome)
	require.NoError(t, ensureDirectories(dirAsHelmHome))

	for _, dir := range getHomeTree(dirAsHelmHome) {
		assert.True(t, files.IsDir(dir), "Expected subdirectory %s was not created", dir)
	}
}

func TestHelmHomeInitializeHelmHomeCreatesDirWithStableChartRepository(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "")
	defer os.RemoveAll(dir)
	require.NoError(t, err)

	err = initializeHelmHome(dir)
	require.NoError(t, err)

	dirAsHelmHome := helmpath.Home(dir)
	verifyRepoFile(t, dirAsHelmHome)
}

// Verify the repo file is created with the stable repository in there
func verifyRepoFile(t *testing.T, dirAsHelmHome helmpath.Home) {
	repoFile := dirAsHelmHome.RepositoryFile()
	repositoryFile, err := repo.LoadRepositoriesFile(repoFile)
	require.NoError(t, err)
	require.Equal(t, len(repositoryFile.Repositories), 1)
	repository := repositoryFile.Repositories[0]
	assert.Equal(t, repository.Name, StableRepositoryName)
	assert.Equal(t, repository.URL, StableRepositoryURL)
}
