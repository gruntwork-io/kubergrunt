package eksawshelper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagExistsInRepo(t *testing.T) {
	t.Parallel()

	region := "us-west-2"
	existingTag := "v1.18.8-eksbuild.1"
	nonExistingTag := "v1.18.8-eksbuild.10"

	token, err := GetDockerLoginToken(region)
	require.NoError(t, err)
	repoDomain := fmt.Sprintf("602401143452.dkr.ecr.%s.amazonaws.com", region)
	tagExists1, err := TagExistsInRepo(token, repoDomain, "eks/kube-proxy", existingTag)
	require.NoError(t, err)
	assert.True(t, tagExists1)

	tagExists2, err := TagExistsInRepo(token, repoDomain, "eks/kube-proxy", nonExistingTag)
	require.NoError(t, err)
	assert.False(t, tagExists2)
}
