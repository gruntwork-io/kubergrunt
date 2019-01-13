package kubectl

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/gruntwork-cli/files"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd/api"
)

type MockEksConfigContextData struct {
	Config      *api.Config
	Name        string
	EksArn      string
	EksName     string
	EksEndpoint string
	EksCAData   string
}

func TestCreateInitialConfigCreatesDir(t *testing.T) {
	// Make sure this is not a relative dir
	dirName := random.UniqueId()
	require.NotEqual(t, dirName, "")
	require.NotEqual(t, dirName, ".")
	require.NotEqual(t, dirName, "..")
	currentDir, err := os.Getwd()
	require.NoError(t, err)
	configPath := filepath.Join(currentDir, dirName, "config")
	defer os.RemoveAll(filepath.Join(currentDir, dirName))
	err = CreateInitialConfig(configPath)
	require.NoError(t, err)
	require.True(t, files.FileExists(configPath))
	kubeconfig := k8s.LoadConfigFromPath(configPath)
	require.NotNil(t, kubeconfig)
}

func TestAddContextToConfig(t *testing.T) {
	mockConfig := api.NewConfig()
	contextName := random.UniqueId()
	clusterName := random.UniqueId()
	authInfoName := random.UniqueId()

	err := AddContextToConfig(mockConfig, contextName, clusterName, authInfoName)
	require.NoError(t, err)

	context, ok := mockConfig.Contexts[contextName]
	require.True(t, ok)
	assert.Equal(t, context.Cluster, clusterName)
	assert.Equal(t, context.AuthInfo, authInfoName)
}

func TestAddEksConfigContextHonorsContextName(t *testing.T) {
	mockData, err := basicAddCall(t)
	require.NoError(t, err)

	context, ok := mockData.Config.Contexts[mockData.Name]
	require.True(t, ok)
	assert.Equal(t, context.Cluster, mockData.EksArn)
	assert.Equal(t, context.AuthInfo, mockData.EksArn)
}

func TestAddEksConfigContextFailsOnAddingExistingContext(t *testing.T) {
	mockConfig := api.NewConfig()
	mockConfig.Contexts[t.Name()] = api.NewContext()
	err := AddEksConfigContext(
		mockConfig,
		t.Name(),
		"",
		"",
		"",
		"",
	)
	err = errors.Unwrap(err)
	require.IsType(t, ContextAlreadyExistsError{}, err, err.Error())
}

func TestAddClusterToConfigAppendsCorrectClusterInfo(t *testing.T) {
	mockConfig := api.NewConfig()
	clusterName := "devops"
	clusterEndpoint := "dev"
	b64CertificateAuthorityData := base64.StdEncoding.EncodeToString([]byte("ops"))

	err := AddClusterToConfig(mockConfig, clusterName, clusterEndpoint, b64CertificateAuthorityData)
	require.NoError(t, err)

	caData, err := base64.StdEncoding.DecodeString(b64CertificateAuthorityData)
	require.NoError(t, err)

	cluster, ok := mockConfig.Clusters[clusterName]
	require.True(t, ok)
	assert.Equal(t, cluster.Server, clusterEndpoint)
	assert.Equal(t, cluster.CertificateAuthorityData, caData)
	assert.False(t, cluster.InsecureSkipTLSVerify)
}

func TestAddEksConfigContextAppendsCorrectClusterInfo(t *testing.T) {
	mockData, err := basicAddCall(t)
	require.NoError(t, err)
	caData, err := base64.StdEncoding.DecodeString(mockData.EksCAData)
	require.NoError(t, err)

	cluster, ok := mockData.Config.Clusters[mockData.EksArn]
	require.True(t, ok)
	assert.Equal(t, cluster.Server, mockData.EksEndpoint)
	assert.Equal(t, cluster.CertificateAuthorityData, caData)
	assert.False(t, cluster.InsecureSkipTLSVerify)
}

func TestAddEksConfigContextAppendsCorrectAuthInfo(t *testing.T) {
	mockData, err := basicAddCall(t)
	require.NoError(t, err)

	authInfo, ok := mockData.Config.AuthInfos[mockData.EksArn]
	require.True(t, ok)

	execInfo := authInfo.Exec
	assert.Equal(t, execInfo.Command, "kubergrunt")
	assert.Contains(t, execInfo.Args, mockData.EksName)

	// Verify none of the other authentication styles are set
	assert.Equal(t, authInfo.ClientCertificate, "")
	assert.Equal(t, authInfo.ClientCertificateData, []byte(nil))
	assert.Equal(t, authInfo.ClientKey, "")
	assert.Equal(t, authInfo.ClientKeyData, []byte(nil))
	assert.Equal(t, authInfo.Token, "")
	assert.Equal(t, authInfo.TokenFile, "")
	assert.Equal(t, authInfo.Impersonate, "")
	assert.Equal(t, authInfo.ImpersonateGroups, []string(nil))
	assert.Equal(t, authInfo.ImpersonateUserExtra, map[string][]string{})
	assert.Equal(t, authInfo.Username, "")
	assert.Equal(t, authInfo.Password, "")
}

func TestAddEksAuthInfoToConfigAppendsCorrectAuthInfo(t *testing.T) {
	mockConfig := api.NewConfig()
	name := t.Name()
	arn := "arn:aws:eks:us-east-2:111111111111:cluster/" + t.Name()

	err := AddEksAuthInfoToConfig(mockConfig, arn, name)
	require.NoError(t, err)

	authInfo, ok := mockConfig.AuthInfos[arn]
	require.True(t, ok)

	execInfo := authInfo.Exec
	assert.Equal(t, execInfo.Command, "kubergrunt")
	assert.Contains(t, execInfo.Args, name)

	// Verify none of the other authentication styles are set
	assert.Equal(t, authInfo.ClientCertificate, "")
	assert.Equal(t, authInfo.ClientCertificateData, []byte(nil))
	assert.Equal(t, authInfo.ClientKey, "")
	assert.Equal(t, authInfo.ClientKeyData, []byte(nil))
	assert.Equal(t, authInfo.Token, "")
	assert.Equal(t, authInfo.TokenFile, "")
	assert.Equal(t, authInfo.Impersonate, "")
	assert.Equal(t, authInfo.ImpersonateGroups, []string(nil))
	assert.Equal(t, authInfo.ImpersonateUserExtra, map[string][]string{})
	assert.Equal(t, authInfo.Username, "")
	assert.Equal(t, authInfo.Password, "")
}

// basicAddCall makes a call to AddEksConfigContext with fake data and returns the mock config, fake data, and if there
// was an error adding the context.
func basicAddCall(t *testing.T) (MockEksConfigContextData, error) {
	uniqueID := random.UniqueId()
	arn := "arn:aws:eks:us-east-2:111111111111:cluster/" + t.Name()
	name := t.Name()
	endpoint := "gruntwork.io"

	anotherUniqueID := random.UniqueId()
	b64CertificateAuthorityData := base64.StdEncoding.EncodeToString([]byte(anotherUniqueID))

	mockConfig := api.NewConfig()
	err := AddEksConfigContext(
		mockConfig,
		uniqueID,
		arn,
		name,
		endpoint,
		b64CertificateAuthorityData,
	)
	mockData := MockEksConfigContextData{
		Config:      mockConfig,
		Name:        uniqueID,
		EksArn:      arn,
		EksName:     name,
		EksEndpoint: endpoint,
		EksCAData:   b64CertificateAuthorityData,
	}
	return mockData, err
}
