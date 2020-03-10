package kubectl

import (
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/gruntwork-io/kubergrunt/logging"
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
		resp, err := servicesApi.List(params)
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

// GetLoadBalancerNames will query Kubernetes for all services, and then parse out the names of the underlying
// external LoadBalancers.
func GetLoadBalancerNames(kubectlOptions *KubectlOptions) ([]string, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Getting all LoadBalancer names from services in kubernetes")

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

	lbNames := []string{}
	for _, service := range loadBalancerServices {
		lbName, err := GetLoadBalancerNameFromService(service)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		lbNames = append(lbNames, lbName)
	}
	logger.Infof("Successfully extracted loadbalancer names")
	return lbNames, nil
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

	// For ELB, the subdomain will be one of NAME-TIME or internal-NAME-TIME
	loadbalancerHostnameSubDomain := strings.Split(loadbalancerHostname, ".")[0]
	loadbalancerHostnameSubDomainParts := strings.Split(loadbalancerHostnameSubDomain, "-")
	numParts := len(loadbalancerHostnameSubDomainParts)
	if numParts == 2 {
		return loadbalancerHostnameSubDomainParts[0], nil
	} else if numParts == 3 {
		return loadbalancerHostnameSubDomainParts[1], nil
	} else {
		return "", NewLoadBalancerNameFormatError(loadbalancerHostname)
	}
}
