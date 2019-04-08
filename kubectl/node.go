package kubectl

import (
	"sync"
	"time"

	"github.com/gruntwork-io/gruntwork-cli/collections"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/gruntwork-io/kubergrunt/logging"
)

// WaitForNodesReady will continuously watch the nodes until they reach the ready state.
func WaitForNodesReady(
	kubectlOptions *KubectlOptions,
	nodeIds []string,
	maxRetries int,
	sleepBetweenRetries time.Duration,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Waiting for %d nodes in Kubernetes to reach ready state", len(nodeIds))

	client, err := GetKubernetesClientFromOptions(kubectlOptions)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	for i := 0; i < maxRetries; i++ {
		logger.Infof("Checking if nodes ready")
		nodes, err := GetNodes(client, metav1.ListOptions{})
		if err != nil {
			return errors.WithStackTrace(err)
		}
		newNodes := filterNodesByID(nodes, nodeIds)
		logger.Debugf("Received %d nodes. Expecting %d nodes.", len(newNodes), len(nodeIds))
		allNewNodesRegistered := len(newNodes) == len(nodeIds)
		allNewNodesReady := allNodesReady(newNodes)
		if allNewNodesRegistered && allNewNodesReady {
			return nil
		}
		if !allNewNodesRegistered {
			logger.Infof("Not all nodes are registered yet")
		}
		if !allNewNodesReady {
			logger.Infof("Not all nodes are ready yet")
		}
		logger.Infof("Waiting for %s...", sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}
	// Time out
	logger.Errorf("Timedout waiting for nodes to reach ready state")
	if err := reportAllNotReadyNodes(client, nodeIds); err != nil {
		return err
	}
	return errors.WithStackTrace(NewNodeReadyTimeoutError(len(nodeIds)))
}

// reportAllNotReadyNodes will log error messages for each node that is not ready
func reportAllNotReadyNodes(client *kubernetes.Clientset, nodeIds []string) error {
	logger := logging.GetProjectLogger()
	nodes, err := GetNodes(client, metav1.ListOptions{})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	filteredNodes := filterNodesByID(nodes, nodeIds)
	for _, node := range filteredNodes {
		if !IsNodeReady(node) {
			logger.Errorf("Node %s is not ready", node.Name)
		}
	}
	return nil
}

// allNodesReady will return true if all the nodes in the list are ready, and false when any node is not.
func allNodesReady(nodes []corev1.Node) bool {
	logger := logging.GetProjectLogger()
	for _, node := range nodes {
		if !IsNodeReady(node) {
			logger.Debugf("Node %s is not ready", node.Name)
			return false
		}
		logger.Debugf("Node %s is ready", node.Name)
	}
	return true
}

// filterNodesByID will return the list of nodes that correspond to the given node id
func filterNodesByID(nodes []corev1.Node, nodeIds []string) []corev1.Node {
	filteredNodes := []corev1.Node{}
	for _, node := range nodes {
		if collections.ListContainsElement(nodeIds, node.Name) {
			filteredNodes = append(filteredNodes, node)
		}
	}
	return filteredNodes
}

// DrainNodes calls `kubectl drain` on each node provided. Draining a node consists of:
// - Taint the nodes so that new pods are not scheduled
// - Evict all the pods gracefully
// See
// https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/#use-kubectl-drain-to-remove-a-node-from-service
// for more information.
func DrainNodes(kubectlOptions *KubectlOptions, nodeIds []string, timeout time.Duration) error {
	// Concurrently trigger drain events for all requested nodes.
	var wg sync.WaitGroup                      // So that we can wait for all the drain calls
	errChannel := make(chan NodeDrainError, 1) // Collect all errors from each command
	for _, nodeID := range nodeIds {
		wg.Add(1)
		go drainNode(&wg, errChannel, kubectlOptions, nodeID, timeout)
	}
	go waitForAllDrains(&wg, errChannel)

	drainErrors := NewNodeDrainErrors()
	for err := range errChannel {
		if err.Error != nil {
			drainErrors.AddError(err)
		}
	}
	if !drainErrors.IsEmpty() {
		return errors.WithStackTrace(drainErrors)
	}
	return nil
}

func drainNode(
	wg *sync.WaitGroup,
	errChannel chan<- NodeDrainError,
	kubectlOptions *KubectlOptions,
	nodeID string,
	timeout time.Duration,
) {
	defer wg.Done()
	err := RunKubectl(kubectlOptions, "drain", nodeID, "--ignore-daemonsets", "--timeout", timeout.String())
	errChannel <- NodeDrainError{NodeID: nodeID, Error: err}
}

func waitForAllDrains(wg *sync.WaitGroup, errChannel chan<- NodeDrainError) {
	wg.Wait()
	close(errChannel)
}

// GetNodes queries Kubernetes for information about the worker nodes registered to the cluster, given a
// clientset.
func GetNodes(clientset *kubernetes.Clientset, options metav1.ListOptions) ([]corev1.Node, error) {
	nodes, err := clientset.CoreV1().Nodes().List(options)
	if err != nil {
		return nil, err
	}
	return nodes.Items, err
}

// IsNodeReady takes a Kubernetes Node information object and checks if the Node is in the ready state.
func IsNodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
