package eks

import (
	"github.com/gruntwork-io/kubergrunt/logging"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseNonExistingDeployState(t *testing.T) {
	t.Parallel()
	fileName := "./.na"
	state, err := initDeployState(fileName, false, 3, 30*time.Second)
	require.NoError(t, err)
	defer os.Remove(fileName)

	assert.Equal(t, fileName, state.Path)
	assert.Equal(t, 3, state.maxRetries)
	assert.Equal(t, 30*time.Second, state.sleepBetweenRetries)
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
	state, err := initDeployState(stateFile, false, 3, 30*time.Second)
	require.NoError(t, err)
	defer os.Remove(stateFile)

	assert.True(t, state.GatherASGInfoDone)
	assert.False(t, state.SetMaxCapacityDone)
	assert.Equal(t, 1, len(state.ASGs))
	assert.Equal(t, 3, state.maxRetries)
	assert.Equal(t, 30*time.Second, state.sleepBetweenRetries)

	asg := state.ASGs[0]

	assert.Equal(t, "my-test-asg", asg.Name)
	assert.Equal(t, int64(2), asg.OriginalCapacity)
	assert.Equal(t, int64(4), asg.OriginalMaxCapacity)
	assert.Equal(t, 2, len(asg.OriginalInstances))
	assert.Equal(t, 1, len(asg.NewInstances))
}

func TestParseExistingDeployStateIgnoreCurrent(t *testing.T) {
	t.Parallel()

	stateFile := generateTempStateFile(t)
	state, err := initDeployState(stateFile, true, 3, 30*time.Second)
	require.NoError(t, err)
	defer os.Remove(stateFile)

	assert.False(t, state.GatherASGInfoDone)
	assert.Equal(t, 0, len(state.ASGs))
}

func generateTempStateFile(t *testing.T) string {
	escapedTestName := url.PathEscape(t.Name())
	tmpfile, err := ioutil.TempFile("", escapedTestName)
	require.NoError(t, err)
	defer tmpfile.Close()

	asg := ASG{
		Name:                "my-test-asg",
		OriginalCapacity:    2,
		OriginalMaxCapacity: 4,
		OriginalInstances: []string{
			"instance-1",
			"instance-2",
		},
		NewInstances: []string{
			"instance-3",
		},
	}

	state := &DeployState{
		logger:            logging.GetProjectLogger(),
		GatherASGInfoDone: true,
		Path:              tmpfile.Name(),
		ASGs:              []ASG{asg},
	}

	state.persist()
	return tmpfile.Name()
}
