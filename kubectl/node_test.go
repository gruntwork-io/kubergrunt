package kubectl

import (
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestWaitForNodesReady(t *testing.T) {
	t.Parallel()
	kubeConfigPath, err := k8s.GetKubeConfigPathE(t)
	require.NoError(t, err)

	node := getNodes(t)[0]
	nodeID := node.Name
	require.NoError(t, WaitForNodesReady(NewKubectlOptions("", kubeConfigPath), []string{nodeID}, 40, 15*time.Second))
}

func TestFilterNodesById(t *testing.T) {
	t.Parallel()
	nodes := getNodes(t)
	require.Equal(t, len(filterNodesByID(nodes, []string{})), 0)
	require.Equal(t, len(filterNodesByID(nodes, []string{nodes[0].Name})), 1)
}

func getNodes(t *testing.T) []corev1.Node {
	nodes := k8s.GetNodes(t)
	// Assumes local kubernetes (minikube or docker-for-desktop kube), where there is only one node
	require.Equal(t, len(nodes), 1)
	return nodes
}
