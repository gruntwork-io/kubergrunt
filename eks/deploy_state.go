package eks

import (
	"github.com/gruntwork-io/kubergrunt/logging"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/json"
	"os"
)

const defaultStateFile = "./.kubergrunt.state"

type DeployState struct {
	GatherASGInfoDone      bool
	SetMaxCapacityDone     bool
	ScaleUpDone            bool
	WaitForNodesDone       bool
	CordonNodesDone        bool
	DrainNodesDone         bool
	DetachInstancesDone    bool
	TerminateInstancesDone bool
	RestoreCapacityDone    bool

	Path string
	ASG  ASG
}

type ASG struct {
	Name                 string
	OriginalCapacity     int64
	MaxCapacityForUpdate int64
	MaxSize              int64
	OriginalInstances    []string
	NewInstances         []string
}

func readOrInitializeState(file string, ignoreExistingFile bool) (*DeployState, error) {
	logger := logging.GetProjectLogger()
	if ignoreExistingFile {
		logger.Info("Ignore existing state file.")
		return newDeployState(file), nil
	}

	logger.Infof("Looking for existing recovery file %s", file)
	data, err := ioutil.ReadFile(file)
	if err != nil {
		logger.Debugf("No state present, creating new: %s", err.Error())
		return newDeployState(file), nil
	}
	var parsedState DeployState
	err = json.Unmarshal(data, &parsedState)
	if err != nil {
		return nil, err
	}
	return &parsedState, nil
}

func (state *DeployState) persist() {
	file := state.Path
	logger := logging.GetProjectLogger()
	logger.Debugf("storing state file %s", file)

	data, err := json.Marshal(state)

	if err != nil {
		logger.Fatalf("Error marshaling state: %v", err)
	}

	err = ioutil.WriteFile(file, data, 0644)
	if err != nil {
		logger.Fatalf("Error storing state to %s: %v", file, err)
	}
}

func (state *DeployState) delete() error {
	file := state.Path
	logger := logging.GetProjectLogger()
	logger.Debugf("Deleting state file %s", file)

	err := os.Remove(file)
	if err != nil {
		return err
	}

	return nil
}

func newDeployState(path string) *DeployState {
	return &DeployState{
		Path: path,
		ASG:  ASG{},
	}
}
