package kubectl

// AWSLoadBalancer is a struct that represents an AWS ELB that is associated with Kubernetes resources (Service or
// Ingress).
type AWSLoadBalancer struct {
	Name       string
	Type       ELBType
	TargetType ELBTargetType
}

// ELBType represents the underlying type of the load balancer (classic, network, or application)
type ELBType int

const (
	ALB ELBType = iota
	NLB
	CLB
	UnknownELB
)

// ELBTargetType represents the different ways the AWS ELB routes to the services.
type ELBTargetType int

const (
	InstanceTarget ELBTargetType = iota
	IPTarget
	UnknownELBTarget
)
