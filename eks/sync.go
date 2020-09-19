package eks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gruntwork-io/gruntwork-cli/collections"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/jsonpatch"
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

var (
	// NOTE: Ensure that there is an entry for each supported version in the following tables.
	supportedVersions = []string{"1.17", "1.16", "1.15", "1.14"}

	coreDNSVersionLookupTable = map[string]string{
		"1.17": "1.6.6-eksbuild.1",
		"1.16": "1.6.6-eksbuild.1",
		"1.15": "1.6.6-eksbuild.1",
		"1.14": "1.6.6-eksbuild.1",
	}

	kubeProxyVersionLookupTable = map[string]string{
		"1.17": "1.17.9-eksbuild.1",
		"1.16": "1.16.13-eksbuild.1",
		"1.15": "1.15.11-eksbuild.1",
		"1.14": "1.14.9-eksbuild.1",
	}

	amazonVPCCNIVersionLookupTable = map[string]string{
		"1.17": "1.6",
		"1.16": "1.6",
		"1.15": "1.6",
		"1.14": "1.6",
	}
)

const (
	componentNamespace     = "kube-system"
	kubeProxyDaemonSetName = "kube-proxy"
	corednsDeploymentName  = "coredns"
)

// SyncClusterComponents will perform the steps described in
// https://docs.aws.amazon.com/eks/latest/userguide/update-cluster.html
// There are three core applications on an EKS cluster:
//
//    - kube-proxy
//    - coredns
//    - VPC CNI Plugin
//
// Each of these are managed in Kubernetes as DaemonSet, Deployment, and DaemonSet respectively. This command will use
// the k8s API and kubectl command under the hood to patch the manifests to deploy the expected version based on what
// the current Kubernetes version is of the cluster. As such, this command should be run every time the Kubernetes
// version is updated on the EKS cluster.
func SyncClusterComponents(
	eksClusterArn string,
	shouldWait bool,
	waitTimeout string,
) error {
	logger := logging.GetProjectLogger()

	logger.Info("Looking up deployed Kubernetes version")
	clusterInfo, err := eksawshelper.GetClusterByArn(eksClusterArn)
	if err != nil {
		return err
	}
	k8sVersion := aws.StringValue(clusterInfo.Version)

	if !collections.ListContainsElement(supportedVersions, k8sVersion) {
		return errors.WithStackTrace(UnsupportedEKSVersion{k8sVersion})
	}

	kubeProxyVersion := kubeProxyVersionLookupTable[k8sVersion]
	coreDNSVersion := coreDNSVersionLookupTable[k8sVersion]
	amznVPCCNIVersion := amazonVPCCNIVersionLookupTable[k8sVersion]
	logger.Info("Syncing Kubernetes Applications to:")
	logger.Infof("\tkube-proxy:\t%s", kubeProxyVersion)
	logger.Infof("\tcoredns:\t%s", coreDNSVersion)
	logger.Infof("\tVPC CNI Plugin:\t%s", amznVPCCNIVersion)

	kubectlOptions := &kubectl.KubectlOptions{EKSClusterArn: eksClusterArn}
	clientset, err := kubectl.GetKubernetesClientFromOptions(kubectlOptions)
	if err != nil {
		return err
	}

	awsRegion, err := eksawshelper.GetRegionFromArn(eksClusterArn)
	if err != nil {
		return err
	}
	if err := upgradeKubeProxy(kubectlOptions, clientset, awsRegion, kubeProxyVersion, shouldWait, waitTimeout); err != nil {
		return err
	}
	if err := upgradeCoreDNS(kubectlOptions, clientset, awsRegion, coreDNSVersion, shouldWait, waitTimeout); err != nil {
		return err
	}
	if err := updateVPCCNI(kubectlOptions, amznVPCCNIVersion); err != nil {
		return err
	}

	logger.Info("Successfully updated core components.")
	return nil
}

// upgradeKubeProxy will update to the latest kube-proxy version if necessary. If shouldWait is set to true, this
// routine will wait until the new images are fully rolled out before continuing.
func upgradeKubeProxy(
	kubectlOptions *kubectl.KubectlOptions,
	clientset *kubernetes.Clientset,
	awsRegion string,
	kubeProxyVersion string,
	shouldWait bool,
	waitTimeout string,
) error {
	logger := logging.GetProjectLogger()

	targetImage := fmt.Sprintf("602401143452.dkr.ecr.%s.amazonaws.com/eks/kube-proxy:v%s", awsRegion, kubeProxyVersion)
	currentImage, err := getCurrentDeployedKubeProxyImage(clientset)
	if err != nil {
		return err
	}
	if currentImage == targetImage {
		logger.Info("Current deployed version matches expected version. Skipping kube-proxy update.")
		return nil
	}

	logger.Infof("Upgrading current deployed version of kube-proxy (%s) to match expected version (%s).", currentImage, targetImage)
	if err := updateKubeProxyDaemonsetImage(clientset, targetImage); err != nil {
		return err
	}
	if shouldWait {
		logger.Info("Waiting until new image for kube-proxy is rolled out.")
		// Ideally we will implement the following routine using the raw client-go library, but implementing this
		// functionality directly on the API is fairly complex, and thus we rely on the built in mechanism in kubectl
		// instead.
		args := []string{
			"rollout",
			"status",
			fmt.Sprintf("daemonset/%s", kubeProxyDaemonSetName),
			"-n", componentNamespace,
			"--timeout", waitTimeout,
		}
		return kubectl.RunKubectl(kubectlOptions, args...)
	}
	return nil
}

// getCurrentDeployedKubeProxyImage will return the currently configured kube-proxy image on the daemonset.
func getCurrentDeployedKubeProxyImage(clientset *kubernetes.Clientset) (string, error) {
	daemonset, err := clientset.AppsV1().DaemonSets(componentNamespace).Get(context.Background(), kubeProxyDaemonSetName, metav1.GetOptions{})
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	daemonsetContainers := daemonset.Spec.Template.Spec.Containers
	if len(daemonsetContainers) != 1 {
		err := CoreComponentUnexpectedConfigurationErr{
			component: "kube-proxy",
			reason:    fmt.Sprintf("unexpected number of containers (%d)", len(daemonsetContainers)),
		}
		return "", errors.WithStackTrace(err)
	}
	return daemonsetContainers[0].Image, nil
}

// updateKubeProxyDaemonsetImage will update the deployed kube-proxy DaemonSet to the specified target container image.
func updateKubeProxyDaemonsetImage(clientset *kubernetes.Clientset, targetImage string) error {
	patch := []jsonpatch.PatchString{
		{
			Op: jsonpatch.ReplaceOp,
			// Patch the first container's image field
			Path:  "/spec/template/spec/containers/0/image",
			Value: targetImage,
		},
	}
	patchOpJson, err := json.Marshal(patch)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	daemonsetAPI := clientset.AppsV1().DaemonSets(componentNamespace)
	if _, err := daemonsetAPI.Patch(context.Background(), kubeProxyDaemonSetName, k8stypes.JSONPatchType, patchOpJson, metav1.PatchOptions{}); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// upgradeCoreDNS will update to the latest coredns version if necessary. If shouldWait is set to true, this routine
// will wait until the new images are fully rolled out before continuing.
func upgradeCoreDNS(
	kubectlOptions *kubectl.KubectlOptions,
	clientset *kubernetes.Clientset,
	awsRegion string,
	coreDNSVersion string,
	shouldWait bool,
	waitTimeout string,
) error {
	logger := logging.GetProjectLogger()

	targetImage := fmt.Sprintf("602401143452.dkr.ecr.%s.amazonaws.com/eks/coredns:v%s", awsRegion, coreDNSVersion)
	currentImage, err := getCurrentDeployedCoreDNSImage(clientset)
	if err != nil {
		return err
	}
	if currentImage == targetImage {
		logger.Info("Current deployed version matches expected version. Skipping coredns update.")
		return nil
	}

	logger.Infof("Upgrading current deployed version of coredns (%s) to match expected version (%s).", currentImage, targetImage)
	if err := updateCoreDNSDeploymentImage(clientset, targetImage); err != nil {
		return err
	}
	if shouldWait {
		logger.Info("Waiting until new image for coredns is rolled out.")
		// Ideally we will implement the following routine using the raw client-go library, but implementing this
		// functionality directly on the API is fairly complex, and thus we rely on the built in mechanism in kubectl
		// instead.
		args := []string{
			"rollout",
			"status",
			fmt.Sprintf("deployment/%s", corednsDeploymentName),
			"-n", componentNamespace,
			"--timeout", waitTimeout,
		}
		return kubectl.RunKubectl(kubectlOptions, args...)
	}
	return nil
}

// getCurrentDeployedCoreDNSImage will return the currently configured coredns image on the deployment.
func getCurrentDeployedCoreDNSImage(clientset *kubernetes.Clientset) (string, error) {
	deployment, err := clientset.AppsV1().Deployments(componentNamespace).Get(context.Background(), corednsDeploymentName, metav1.GetOptions{})
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	deploymentContainers := deployment.Spec.Template.Spec.Containers
	if len(deploymentContainers) != 1 {
		err := CoreComponentUnexpectedConfigurationErr{
			component: "coredns",
			reason:    fmt.Sprintf("unexpected number of containers (%d)", len(deploymentContainers)),
		}
		return "", errors.WithStackTrace(err)
	}
	return deploymentContainers[0].Image, nil
}

// updateCoreDNSDeploymentImage will update the deployed coredns Deployment to the specified target container image.
func updateCoreDNSDeploymentImage(clientset *kubernetes.Clientset, targetImage string) error {
	patch := []jsonpatch.PatchString{
		{
			Op: jsonpatch.ReplaceOp,
			// Patch the first container's image field
			Path:  "/spec/template/spec/containers/0/image",
			Value: targetImage,
		},
	}
	patchOpJson, err := json.Marshal(patch)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	deploymentAPI := clientset.AppsV1().Deployments(componentNamespace)
	if _, err := deploymentAPI.Patch(context.Background(), corednsDeploymentName, k8stypes.JSONPatchType, patchOpJson, metav1.PatchOptions{}); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// updateVPCCNI will apply the manifest to deploy the latest patch release of the target AWS VPC CNI version. Ideally we
// would implement this using the raw Kubernetes API, but the CNI manifest contains additional resources on top of the
// daemonset, and thus it is better to apply the manifests directly using kubectl than to translate it into underlying
// API calls.
func updateVPCCNI(kubectlOptions *kubectl.KubectlOptions, vpcCNIVersion string) error {
	manifestURL := fmt.Sprintf("https://raw.githubusercontent.com/aws/amazon-vpc-cni-k8s/release-%s/config/v%s/aws-k8s-cni.yaml", vpcCNIVersion, vpcCNIVersion)
	return kubectl.RunKubectl(kubectlOptions, "apply", "-f", manifestURL)
}
