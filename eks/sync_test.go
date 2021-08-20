package eks

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetBaseURLForVPCCNIManifest(t *testing.T) {
	t.Parallel()
	expected := "https://raw.githubusercontent.com/aws/amazon-vpc-cni-k8s/v1.7.5/config/v1.7/"
	actual, err := getBaseURLForVPCCNIManifest("1.7.5")
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

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

func TestSemverStringCompare(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		v1             string
		v2             string
		expectedResult int
		expectErr      bool
	}{
		{
			"GreaterPatch",
			"1.7.10",
			"1.7.9",
			1,
			false,
		},
		{
			"GreaterMinor",
			"1.7.10",
			"1.6.99999",
			1,
			false,
		},
		{
			"LesserPatch",
			"1.7.9",
			"1.7.10",
			-1,
			false,
		},
		{
			"LesserMinor",
			"1.6.99999",
			"1.7.10",
			-1,
			false,
		},
		{
			"Equal",
			"1.7.0",
			"1.7.0",
			0,
			false,
		},
		{
			"CompareAnnotations",
			"1.7.0-eksbuild.1",
			"1.7.0-eksbuild.2",
			-1,
			false,
		},
		{
			"SameVersionWithAnnotation",
			"1.7.0",
			"1.7.0-eksbuild.1",
			1,
			false,
		},
		{
			"OlderVersionWithAnnotation",
			"1.7.0",
			"1.6.0-eksbuild.1",
			1,
			false,
		},
		{
			"BadV1",
			"foo",
			"1.7.0",
			0,
			true,
		},
		{
			"BadV2",
			"1.7.0",
			"foo",
			0,
			true,
		},
	}

	for _, tc := range testCases {
		// Capture range variable
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			compareVal, err := semverStringCompare(tc.v1, tc.v2)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, compareVal, tc.expectedResult)
			}
		})
	}
}

func TestRemoveUpstreamKeywordFromCorednsConfigMap(t *testing.T) {
	t.Parallel()

	namespace := strings.ToLower(random.UniqueId())

	// Create a new namespace, and defer clean up steps.
	kubectlOptions := k8s.NewKubectlOptions("", "", namespace)
	defer k8s.DeleteNamespace(t, kubectlOptions, namespace)
	k8s.CreateNamespace(t, kubectlOptions, namespace)

	// Create the test configmap with coredns config data that needs to be updated.
	clientset, err := k8s.GetKubernetesClientFromOptionsE(t, kubectlOptions)
	require.NoError(t, err)
	configmapAPI := clientset.CoreV1().ConfigMaps(namespace)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: corednsConfigMapName},
		Data:       map[string]string{corednsConfigMapConfigKey: sampleConfigData},
	}
	_, err = configmapAPI.Create(context.Background(), configMap, metav1.CreateOptions{})
	require.NoError(t, err)

	// Apply updates
	testConfigMap, err := configmapAPI.Get(context.Background(), corednsConfigMapName, metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, removeUpstreamKeywordFromCorednsConfigMap(clientset, testConfigMap))

	// Get the latest state of the configmap and verify the data is updated correctly
	testConfigMapUpdated, err := configmapAPI.Get(context.Background(), corednsConfigMapName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, expectedSampleConfigData, testConfigMapUpdated.Data[corednsConfigMapConfigKey])
}

func TestFindLatestEKSBuild(t *testing.T) {
	t.Parallel()

	testCase := []struct {
		k8sVersion      string
		region          string
		expectedVersion string
	}{
		{"1.20", "us-east-1", "1.20.4-eksbuild.2"},
		{"1.16", "us-east-1", "1.16.13-eksbuild.1"},
	}

	for _, tc := range testCase {
		tc := tc
		t.Run(tc.k8sVersion, func(t *testing.T) {
			t.Parallel()

			repoDomain := getRepoDomain(tc.region)
			dockerToken, err := eksawshelper.GetDockerLoginToken(tc.region)
			require.NoError(t, err)

			kubeProxyVersion, err := findLatestEKSBuild(dockerToken, repoDomain, kubeProxyRepoPath, kubeProxyVersionLookupTable[tc.k8sVersion])
			require.NoError(t, err)
			assert.Equal(t, tc.expectedVersion, kubeProxyVersion)
		})
	}
}

const sampleConfigData = `.:53 {
    errors
    health
    kubernetes cluster.local in-addr.arpa ip6.arpa {
      pods insecure
      upstream
      fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}
`

const expectedSampleConfigData = `.:53 {
    errors
    health
    kubernetes cluster.local in-addr.arpa ip6.arpa {
      pods insecure
      fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}
`
