package eks

import (
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNonExistingDeployState(t *testing.T) {
	t.Parallel()
	fileName := "./.na"
	state, err := readOrInitializeState(fileName, false)
	require.NoError(t, err)
	defer os.Remove(fileName)

	assert.Equal(t, fileName, state.Path)

	assert.False(t, state.SetMaxCapacityDone)
	assert.False(t, state.TerminateInstancesDone)
	assert.False(t, state.GatherASGInfoDone)
	assert.False(t, state.RestoreCapacityDone)
	assert.False(t, state.DrainNodesDone)
	assert.False(t, state.CordonNodesDone)
	assert.False(t, state.DetachInstancesDone)
	assert.False(t, state.WaitForNodesDone)
	assert.False(t, state.ScaleUpDone)
}

func TestParseExistingDeployState(t *testing.T) {
	t.Parallel()

	stateFile := generateTempStateFile(t)
	state, err := readOrInitializeState(stateFile, false)
	require.NoError(t, err)
	defer os.Remove(stateFile)

	assert.True(t, state.GatherASGInfoDone)
	assert.False(t, state.SetMaxCapacityDone)
	assert.Equal(t, "my-test-asg", state.ASG.Name)
	assert.Equal(t, int64(2), state.ASG.OriginalCapacity)
	assert.Equal(t, int64(4), state.ASG.MaxSize)
	assert.Equal(t, 2, len(state.ASG.OriginalInstances))
	assert.Equal(t, 1, len(state.ASG.NewInstances))
}

func TestParseExistingDeployStateIgnoreCurrent(t *testing.T) {
	t.Parallel()

	stateFile := generateTempStateFile(t)
	state, err := readOrInitializeState(stateFile, true)
	require.NoError(t, err)
	defer os.Remove(stateFile)

	assert.False(t, state.GatherASGInfoDone)
	assert.Nil(t, state.ASG.OriginalInstances)
	assert.Nil(t, state.ASG.NewInstances)
}

func generateTempStateFile(t *testing.T) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()

	state := &DeployState{
		GatherASGInfoDone: true,
		Path:              tmpfile.Name(),
		ASG: ASG{
			Name:             "my-test-asg",
			OriginalCapacity: 2,
			MaxSize:          4,
			OriginalInstances: []string{
				"instance-1",
				"instance-2",
			},
			NewInstances: []string{
				"instance-3",
			},
		},
	}

	state.persist()
	return tmpfile.Name()
}
