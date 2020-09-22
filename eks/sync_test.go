package eks

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadVPCCNIManifestAndUpdateRegion(t *testing.T) {
	t.Parallel()

	workingDir, err := ioutil.TempDir("", "kubergrunt-sync")
	require.NoError(t, err)
	defer os.RemoveAll(workingDir)
	manifestPath := filepath.Join(workingDir, "aws-k8s-cni.yaml")

	require.NoError(
		t,
		downloadVPCCNIManifestAndUpdateRegion(
			"https://raw.githubusercontent.com/aws/amazon-vpc-cni-k8s/f5bac1d9ff4b7261d44d50705f3657b65f9dbdc5/config/v1.5/aws-k8s-cni.yaml",
			manifestPath,
			"ap-northeast-1",
		),
	)

	// Compare the downloaded file against the fixture
	expectedF, err := ioutil.ReadFile(filepath.Join(".", "fixture", "aws-k8s-cni.yaml"))
	require.NoError(t, err)
	actualF, err := ioutil.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, expectedF, actualF)
}
