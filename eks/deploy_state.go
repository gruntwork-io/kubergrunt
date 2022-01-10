package eks

import (
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/json"
	"os"
	"strings"
	"time"
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
	ASGs []ASG

	maxRetries          int
	sleepBetweenRetries time.Duration

	logger *logrus.Entry
}

type ASG struct {
	Name                 string
	OriginalCapacity     int64
	MaxCapacityForUpdate int64
	OriginalMaxCapacity  int64
	OriginalInstances    []string
	NewInstances         []string
}

func initDeployState(file string, ignoreExistingFile bool, maxRetries int, sleepBetweenRetries time.Duration) (*DeployState, error) {
	logger := logging.GetProjectLogger()
	var deployState *DeployState

	if ignoreExistingFile {
		logger.Info("Ignore existing state file.")
		deployState = newDeployState(file)
	} else {
		logger.Debugf("Looking for existing recovery file %s", file)
		data, err := ioutil.ReadFile(file)
		if err != nil {
			logger.Debugf("No state present, creating new: %s", err.Error())
			deployState = newDeployState(file)
		} else {
			var parsedState DeployState
			err = json.Unmarshal(data, &parsedState)
			if err != nil {
				return nil, err
			}
			deployState = &parsedState
		}
	}

	deployState.logger = logger
	deployState.maxRetries = maxRetries
	deployState.sleepBetweenRetries = sleepBetweenRetries

	return deployState, nil
}

func (state *DeployState) persist() error {
	file := state.Path
	state.logger.Debugf("storing state file %s", file)

	data, err := json.Marshal(state)

	if err != nil {
		return errors.WithStackTrace(err)
	}

	err = ioutil.WriteFile(file, data, 0644)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

func (state *DeployState) delete() error {
	file := state.Path
	state.logger.Debugf("Deleting state file %s", file)

	err := os.Remove(file)

	if err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func newDeployState(path string) *DeployState {
	return &DeployState{
		Path: path,
		ASGs: []ASG{},
	}
}

func (state *DeployState) gatherASGInfo(asgSvc *autoscaling.AutoScaling, eksAsgNames []string) error {
	// If we're in the initial state, gather ASG info and wait for capacity
	if !state.GatherASGInfoDone {
		eksAsgName := eksAsgNames[0]
		// Retrieve the ASG object and gather required info we will need later
		tmpAsgInfo, err := getAsgInfo(asgSvc, eksAsgName)
		if err != nil {
			return err
		}

		// Calculate default max retries
		if state.maxRetries == 0 {
			maxRetries := getDefaultMaxRetries(tmpAsgInfo.originalCapacity, state.sleepBetweenRetries)
			state.logger.Infof(
				"No max retries set. Defaulted to %d based on sleep between retries duration of %s and scale up count %d.",
				maxRetries,
				state.sleepBetweenRetries,
				tmpAsgInfo.originalCapacity,
			)
		}

		// Make sure ASG is in steady state
		if tmpAsgInfo.originalCapacity != int64(len(tmpAsgInfo.currentInstanceIDs)) {
			state.logger.Infof("Ensuring ASG is in steady state (current capacity = desired capacity)")
			err = waitForCapacity(asgSvc, eksAsgName, state.maxRetries, state.sleepBetweenRetries)
			if err != nil {
				state.logger.Error("Error waiting for ASG to reach steady state. Try again after the ASG is in a steady state.")
				return err
			}
			state.logger.Infof("Verified ASG is in steady state (current capacity = desired capacity)")
			tmpAsgInfo, err = getAsgInfo(asgSvc, eksAsgName)
			if err != nil {
				return err
			}
		}

		state.GatherASGInfoDone = true
		asgDetails := ASG{}
		asgDetails.OriginalMaxCapacity = tmpAsgInfo.maxSize
		asgDetails.Name = eksAsgName
		asgDetails.OriginalCapacity = tmpAsgInfo.originalCapacity
		asgDetails.OriginalInstances = tmpAsgInfo.currentInstanceIDs
		state.ASGs = append(state.ASGs, asgDetails)
		return state.persist()
	}
	state.logger.Debug("ASG Info already gathered - skipping")
	return nil
}

func (state *DeployState) setMaxCapacity(asgSvc *autoscaling.AutoScaling) error {
	if !state.SetMaxCapacityDone {
		asg := state.ASGs[0]
		maxCapacityForUpdate := asg.OriginalCapacity * 2
		if asg.OriginalMaxCapacity < maxCapacityForUpdate {
			err := setAsgMaxSize(asgSvc, asg.Name, maxCapacityForUpdate)
			if err != nil {
				return err
			}
		}
		asg.MaxCapacityForUpdate = maxCapacityForUpdate
		state.SetMaxCapacityDone = true
		return state.persist()
	}
	state.logger.Debug("Max capacity already set - skipping")
	return nil
}

func (state *DeployState) scaleUp(asgSvc *autoscaling.AutoScaling) error {
	if !state.ScaleUpDone {
		asg := state.ASGs[0]
		state.logger.Info("Starting with the following list of instances in ASG:")
		state.logger.Infof("%s", strings.Join(asg.OriginalInstances, ","))

		state.logger.Infof("Launching new nodes with new launch config on ASG %s", asg.Name)
		newInstanceIds, err := scaleUp(asgSvc, asg.Name, asg.OriginalInstances, asg.MaxCapacityForUpdate, state.maxRetries, state.sleepBetweenRetries)
		if err != nil {
			return err
		}
		state.logger.Infof("Successfully launched new nodes with new launch config on ASG %s", asg.Name)
		state.ScaleUpDone = true
		asg.NewInstances = newInstanceIds
		return state.persist()
	}
	state.logger.Debug("Scale up already done - skipping")
	return nil
}

func (state *DeployState) waitForNodes(ec2Svc *ec2.EC2, elbSvc *elb.ELB, elbv2Svc *elbv2.ELBV2, kubectlOptions *kubectl.KubectlOptions) error {
	if !state.WaitForNodesDone {
		asg := state.ASGs[0]
		err := waitAndVerifyNewInstances(ec2Svc, elbSvc, elbv2Svc, asg.NewInstances, kubectlOptions, state.maxRetries, state.sleepBetweenRetries)
		if err != nil {
			state.logger.Errorf("Error while waiting for new nodes to be ready.")
			state.logger.Errorf("Either resume with the recovery file or terminate the new instances.")
			return err
		}
		state.WaitForNodesDone = true
		return state.persist()
	}
	state.logger.Debug("Wait for nodes already done - skipping")
	return nil
}

func (state *DeployState) cordonNodes(ec2Svc *ec2.EC2, kubectlOptions *kubectl.KubectlOptions) error {
	if !state.CordonNodesDone {
		asg := state.ASGs[0]
		state.logger.Infof("Cordoning old instances in cluster ASG %s to prevent Pod scheduling", asg.Name)
		err := cordonNodesInAsg(ec2Svc, kubectlOptions, asg.OriginalInstances)
		if err != nil {
			state.logger.Errorf("Error while cordoning nodes.")
			state.logger.Errorf("Either resume with the recovery file or continue to cordon nodes that failed manually, and then terminate the underlying instances to complete the rollout.")
			return err
		}
		state.logger.Infof("Successfully cordoned old instances in cluster ASG %s", asg.Name)
		state.CordonNodesDone = true
		return state.persist()
	}
	state.logger.Debug("Nodes already cordoned - skipping")
	return nil
}

func (state *DeployState) drainNodes(ec2Svc *ec2.EC2, kubectlOptions *kubectl.KubectlOptions, drainTimeout time.Duration, deleteLocalData bool) error {
	if !state.DrainNodesDone {
		asg := state.ASGs[0]
		state.logger.Infof("Draining Pods on old instances in cluster ASG %s", asg.Name)
		err := drainNodesInAsg(ec2Svc, kubectlOptions, asg.OriginalInstances, drainTimeout, deleteLocalData)
		if err != nil {
			state.logger.Errorf("Error while draining nodes.")
			state.logger.Errorf("Either resume with the recovery file or continue to drain nodes that failed manually, and then terminate the underlying instances to complete the rollout.")
			return err
		}
		state.logger.Infof("Successfully drained all scheduled Pods on old instances in cluster ASG %s", asg.Name)
		state.DrainNodesDone = true
		return state.persist()
	}
	state.logger.Debug("Nodes already drained - skipping")
	return nil
}

func (state *DeployState) detachInstances(asgSvc *autoscaling.AutoScaling) error {
	if !state.DetachInstancesDone {
		asg := state.ASGs[0]
		state.logger.Infof("Removing old nodes from ASG %s: %s", asg.Name, strings.Join(asg.OriginalInstances, ","))
		err := detachInstances(asgSvc, asg.Name, asg.OriginalInstances)
		if err != nil {
			state.logger.Errorf("Error while detaching the old instances.")
			state.logger.Errorf("Either resume with the recovery file or continue to detach the old instances and then terminate the underlying instances to complete the rollout.")
			return err
		}
		state.DetachInstancesDone = true
		return state.persist()
	}
	state.logger.Debug("Instances already detached - skipping")
	return nil
}

func (state *DeployState) terminateInstances(ec2Svc *ec2.EC2) error {
	if !state.TerminateInstancesDone {
		asg := state.ASGs[0]
		state.logger.Infof("Terminating old nodes: %s", strings.Join(asg.OriginalInstances, ","))
		err := terminateInstances(ec2Svc, asg.OriginalInstances)
		if err != nil {
			state.logger.Errorf("Error while terminating the old instances.")
			state.logger.Errorf("Either resume with the recovery file or continue to terminate the underlying instances to complete the rollout.")
			return err
		}
		state.logger.Infof("Successfully removed old nodes from ASG %s", asg.Name)
		state.TerminateInstancesDone = true
		return state.persist()
	}
	state.logger.Debug("Instances already terminated - skipping")
	return nil
}

func (state *DeployState) restoreCapacity(asgSvc *autoscaling.AutoScaling) error {
	if !state.RestoreCapacityDone {
		asg := state.ASGs[0]
		err := setAsgMaxSize(asgSvc, asg.Name, asg.OriginalMaxCapacity)
		if err != nil {
			state.logger.Errorf("Error while restoring ASG %s max size to %v.", asg.Name, asg.OriginalMaxCapacity)
			state.logger.Errorf("Either resume with the recovery file or adjust ASG max size manually to complete the rollout.")
			return state.persist()
		}
	}
	state.logger.Debug("Capacity already restored - skipping")
	return nil
}

// Retrieves current state of the ASG and returns the original Capacity and the IDs of the instances currently
// associated with it.
func getAsgInfo(asgSvc *autoscaling.AutoScaling, asgName string) (asgInfo, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Retrieving current ASG info")
	asg, err := GetAsgByName(asgSvc, asgName)
	if err != nil {
		return asgInfo{}, err
	}
	originalCapacity := *asg.DesiredCapacity
	maxSize := *asg.MaxSize
	currentInstances := asg.Instances
	currentInstanceIDs := idsFromAsgInstances(currentInstances)
	logger.Infof("Successfully retrieved current ASG info.")
	logger.Infof("\tCurrent desired capacity: %d", originalCapacity)
	logger.Infof("\tCurrent max size: %d", maxSize)
	logger.Infof("\tCurrent capacity: %d", len(currentInstances))
	return asgInfo{originalCapacity: originalCapacity, maxSize: maxSize, currentInstanceIDs: currentInstanceIDs}, nil
}
