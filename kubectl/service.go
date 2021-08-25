package kubectl

import (
	"context"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/gruntwork-io/kubergrunt/logging"
)

const (
	lbTypeAnnotationKey   = "service.beta.kubernetes.io/aws-load-balancer-type"
	lbTargetAnnotationKey = "service.beta.kubernetes.io/aws-load-balancer-nlb-target-type"

	lbTypeAnnotationNLB        = "nlb"
	lbTypeAnnotationExternal   = "external"
	lbTargetAnnotationIP       = "ip"
	lbTargetAnnotationNLBIP    = "nlb-ip"
	lbTargetAnnotationInstance = "instance"
)

// GetAllServices queries Kubernetes for information on all deployed Service resources in the current cluster that the
// provided client can access.
func GetAllServices(clientset *kubernetes.Clientset) ([]corev1.Service, error) {
	// We use the empty string for the namespace to indicate all namespaces
	namespace := ""
	servicesApi := clientset.CoreV1().Services(namespace)

	services := []corev1.Service{}
	params := metav1.ListOptions{}
	for {
		resp, err := servicesApi.List(context.Background(), params)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		for _, service := range resp.Items {
			services = append(services, service)
		}
		if resp.Continue == "" {
			break
		}
		params.Continue = resp.Continue
	}
	return services, nil
}

// GetAWSLoadBalancers will query Kubernetes for all services, filter for LoadBalancer services, and then parse out the
// following information:
// - Type of LB (NLB or Classic LB)
// - Instance target or IP target
// TODO: support ALBs with Ingress as well
func GetAWSLoadBalancers(kubectlOptions *KubectlOptions) ([]AWSLoadBalancer, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Getting all LoadBalancers from services in kubernetes")

	client, err := GetKubernetesClientFromOptions(kubectlOptions)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	services, err := GetAllServices(client)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	loadBalancerServices := filterLoadBalancerServices(services)
	logger.Infof("Found %d LoadBalancer services of %d services in kubernetes.", len(loadBalancerServices), len(services))

	lbs := []AWSLoadBalancer{}
	for _, service := range loadBalancerServices {
		lbName, err := GetLoadBalancerNameFromService(service)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		lbType, lbTargetType, err := GetLoadBalancerTypeFromService(service)
		if err != nil {
			return nil, err
		}
		lbs = append(
			lbs,
			AWSLoadBalancer{
				Name:       lbName,
				Type:       lbType,
				TargetType: lbTargetType,
			},
		)
	}
	logger.Infof("Successfully extracted AWS Load Balancers")
	return lbs, nil
}

// filterLoadBalancerServices will return services that are of type LoadBalancer from the provided list of services.
func filterLoadBalancerServices(services []corev1.Service) []corev1.Service {
	out := []corev1.Service{}
	for _, service := range services {
		if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			out = append(out, service)
		}
	}
	return out
}

// GetLoadBalancerNameFromService will return the name of the LoadBalancer given a Kubernetes service object
func GetLoadBalancerNameFromService(service corev1.Service) (string, error) {
	loadbalancerInfo := service.Status.LoadBalancer.Ingress
	if len(loadbalancerInfo) == 0 {
		return "", NewLoadBalancerNotReadyError(service.Name)
	}
	loadbalancerHostname := loadbalancerInfo[0].Hostname

	// TODO: When expanding to GCP, update this logic
	return getAWSLoadBalancerNameFromHostname(loadbalancerHostname)
}

// getAWSLoadBalancerNameFromHostname will return the AWS LoadBalancer name given the assigned hostname. For ELB (both
// v1 and v2), the subdomain will be one of NAME-TIME or internal-NAME-TIME. Note that we need to use strings.Join here
// to account for LB names that contain '-'.
func getAWSLoadBalancerNameFromHostname(hostname string) (string, error) {
	loadbalancerHostnameSubDomain := strings.Split(hostname, ".")[0]
	loadbalancerHostnameSubDomainParts := strings.Split(loadbalancerHostnameSubDomain, "-")
	numParts := len(loadbalancerHostnameSubDomainParts)
	if numParts < 2 {
		return "", NewLoadBalancerNameFormatError(hostname)
	} else if loadbalancerHostnameSubDomainParts[0] == "internal" {
		return strings.Join(loadbalancerHostnameSubDomainParts[1:numParts-1], "-"), nil
	} else {
		return strings.Join(loadbalancerHostnameSubDomainParts[:numParts-1], "-"), nil
	}
}

// GetLoadBalancerTypeFromService will return the ELB type and target type of the given LoadBalancer Service. This uses
// the following heuristic:
// - A LoadBalancer Service with no type annotations will default to Classic Load Balancer (from the in-tree
//   controller).
// - If service.beta.kubernetes.io/aws-load-balancer-type is set to nlb or external, then the ELB will be NLB. (When
//   external, we assume the LB controller handles it)
// - For LB services handled by the LB controller, also check for
//   service.beta.kubernetes.io/aws-load-balancer-nlb-target-type which determines the target type. Otherwise, it is
//   always instance target type.
func GetLoadBalancerTypeFromService(service corev1.Service) (ELBType, ELBTargetType, error) {
	annotations := service.ObjectMeta.Annotations
	lbTypeString, hasLBTypeAnnotation := annotations[lbTypeAnnotationKey]
	if !hasLBTypeAnnotation {
		// No annotation base case
		return CLB, InstanceTarget, nil
	}

	if lbTypeString == lbTypeAnnotationNLB {
		// in-tree controller based NLB provisioning only supports instance targets
		return NLB, InstanceTarget, nil
	} else if lbTypeString != lbTypeAnnotationExternal {
		// Unsupported load balancer type
		return UnknownELB, UnknownELBTarget, errors.WithStackTrace(UnknownAWSLoadBalancerTypeErr{typeKey: lbTypeAnnotationKey, typeStr: lbTypeString})
	}

	// lbTypeString is external at this point, which means we are using the AWS LB controller. This means we need to
	// take into account the target type.
	lbTargetTypeString, hasLBTargetAnnotation := annotations[lbTargetAnnotationKey]
	if !hasLBTargetAnnotation {
		// Default is instance target type
		return NLB, InstanceTarget, nil
	}
	switch lbTargetTypeString {
	case lbTargetAnnotationInstance:
		return NLB, InstanceTarget, nil
	case lbTargetAnnotationIP, lbTargetAnnotationNLBIP:
		return NLB, IPTarget, nil
	default:
		return NLB, UnknownELBTarget, errors.WithStackTrace(UnknownAWSLoadBalancerTypeErr{typeKey: lbTargetAnnotationKey, typeStr: lbTargetTypeString})
	}
}
