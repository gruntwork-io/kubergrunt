package kubectl

import (
	"fmt"
)

// KubeContextNotFound error is returned when the specified Kubernetes context is unabailable in the specified
// kubeconfig.
type KubeContextNotFound struct {
	Options *KubectlOptions
}

func (err KubeContextNotFound) Error() string {
	return fmt.Sprintf("Context %s does not exist in config %s", err.Options.ContextName, err.Options.ConfigPath)
}

// ContextAlreadyExistsError is returned when trying to create a new context with a name that is already in the config
type ContextAlreadyExistsError struct {
	contextName string
}

func (err ContextAlreadyExistsError) Error() string {
	return fmt.Sprintf("kubeconfig context %s already exists", err.contextName)
}

func NewContextAlreadyExistsError(contextName string) ContextAlreadyExistsError {
	return ContextAlreadyExistsError{contextName}
}

// AuthSchemeNotSupported is returned when the specified auth scheme in KubectlOptions is not supported.
type AuthSchemeNotSupported struct {
	scheme AuthScheme
}

func (err AuthSchemeNotSupported) Error() string {
	return fmt.Sprintf("The auth scheme %s is not supported", authSchemeToString(err.scheme))
}

// NodeReadyTimeoutError is returned when we timeout waiting for nodes to reach ready state
type NodeReadyTimeoutError struct {
	numNodes int
}

func (err NodeReadyTimeoutError) Error() string {
	return fmt.Sprintf("Timed out wiating for %d nodes to reach ready state", err.numNodes)
}

func NewNodeReadyTimeoutError(numNodes int) NodeReadyTimeoutError {
	return NodeReadyTimeoutError{numNodes}
}

// NodeDrainError is returned when there is an error draining a node.
type NodeDrainError struct {
	Error  error
	NodeID string
}

// NodeDrainErrors is returned when there are errors draining nodes concurrently. Each node that has an error is added
// to the list.
type NodeDrainErrors struct {
	errors []NodeDrainError
}

func (err NodeDrainErrors) Error() string {
	base := "Multiple errors caught while draining a node:\n"
	for _, subErr := range err.errors {
		subErrMessage := fmt.Sprintf("Node %s: %s", subErr.NodeID, subErr.Error)
		base = base + subErrMessage + "\n"
	}
	return base
}

func (err NodeDrainErrors) AddError(newErr NodeDrainError) {
	err.errors = append(err.errors, newErr)
}

func (err NodeDrainErrors) IsEmpty() bool {
	return len(err.errors) == 0
}

func NewNodeDrainErrors() NodeDrainErrors {
	return NodeDrainErrors{[]NodeDrainError{}}
}

// NodeCordonError is returned when there is an error cordoning a node.
type NodeCordonError struct {
	Error  error
	NodeID string
}

// NodeCordonErrors is returned when there are errors cordoning nodes concurrently. Each node that has an error is added
// to the list.
type NodeCordonErrors struct {
	errors []NodeCordonError
}

func (err NodeCordonErrors) Error() string {
	base := "Multiple errors caught while cordoning nodes:\n"
	for _, subErr := range err.errors {
		subErrMessage := fmt.Sprintf("Node %s: %s", subErr.NodeID, subErr.Error)
		base = base + subErrMessage + "\n"
	}
	return base
}

func (err NodeCordonErrors) AddError(newErr NodeCordonError) {
	err.errors = append(err.errors, newErr)
}

func (err NodeCordonErrors) IsEmpty() bool {
	return len(err.errors) == 0
}

func NewNodeCordonErrors() NodeCordonErrors {
	return NodeCordonErrors{[]NodeCordonError{}}
}

// LoadBalancerNotReadyError is returned when the LoadBalancer Service is unexpectedly not ready.
type LoadBalancerNotReadyError struct {
	serviceName string
}

func (err LoadBalancerNotReadyError) Error() string {
	return fmt.Sprintf("LoadBalancer is not ready on service %s", err.serviceName)
}

func NewLoadBalancerNotReadyError(serviceName string) LoadBalancerNotReadyError {
	return LoadBalancerNotReadyError{serviceName}
}

// LoadBalancerNameFormatError is returned when the hostname of the load balancer is in an unexpected format
type LoadBalancerNameFormatError struct {
	hostname string
}

func (err LoadBalancerNameFormatError) Error() string {
	return fmt.Sprintf("LoadBalancer hostname is in an unexpected format: %s", err.hostname)
}

func NewLoadBalancerNameFormatError(hostname string) LoadBalancerNameFormatError {
	return LoadBalancerNameFormatError{hostname}
}

// ProvisionIngressEndpointTimeoutError is returned when we time out waiting for the endpoint to be provisioned.
type ProvisionIngressEndpointTimeoutError struct {
	ingressName string
	namespace   string
}

func (err ProvisionIngressEndpointTimeoutError) Error() string {
	return fmt.Sprintf(
		"Timed out waiting for Ingress %s (Namespace: %s) to provision endpoint.",
		err.ingressName,
		err.namespace,
	)
}
