package eks

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/blang/semver/v4"
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
	supportedVersions = []string{"1.18", "1.17", "1.16", "1.15", "1.14"}

	coreDNSVersionLookupTable = map[string]string{
		"1.18": "1.7.0-eksbuild.1",
		"1.17": "1.6.6-eksbuild.1",
		"1.16": "1.6.6-eksbuild.1",
		"1.15": "1.6.6-eksbuild.1",
		"1.14": "1.6.6-eksbuild.1",
	}

	kubeProxyVersionLookupTable = map[string]string{
		"1.18": "1.18.8-eksbuild.1",
		"1.17": "1.17.9-eksbuild.1",
		"1.16": "1.16.13-eksbuild.1",
		"1.15": "1.15.11-eksbuild.1",
		"1.14": "1.14.9-eksbuild.1",
	}

	amazonVPCCNIVersionLookupTable = map[string]string{
		"1.18": "1.7.5",
		"1.17": "1.7.5",
		"1.16": "1.7.5",
		"1.15": "1.7.5",
		"1.14": "1.7.5",
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
	if err := updateVPCCNI(kubectlOptions, awsRegion, amznVPCCNIVersion); err != nil {
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

// getBaseURLForVPCCNIManifest returns the base github URL where the manifest for the VPC CNI is located given the
// requested version.
func getBaseURLForVPCCNIManifest(vpcCNIVersion string) (string, error) {
	// Extract the major and minor version of the VPC CNI version as it is needed to construct the URL for the
	// deployment config.
	parsedVPCCNIVersion, err := semver.Make(vpcCNIVersion)
	if err != nil {
		return "", err
	}
	majorMinorVPCCNIVersion := fmt.Sprintf("%d.%d", parsedVPCCNIVersion.Major, parsedVPCCNIVersion.Minor)
	baseURL := fmt.Sprintf("https://raw.githubusercontent.com/aws/amazon-vpc-cni-k8s/v%s/config/v%s/", vpcCNIVersion, majorMinorVPCCNIVersion)
	return baseURL, nil
}

// updateVPCCNI will apply the manifest to deploy the latest patch release of the target AWS VPC CNI version. Ideally we
// would implement this using the raw Kubernetes API, but the CNI manifest contains additional resources on top of the
// daemonset, and thus it is better to apply the manifests directly using kubectl than to translate it into underlying
// API calls.
func updateVPCCNI(kubectlOptions *kubectl.KubectlOptions, region string, vpcCNIVersion string) error {
	var manifestPath string

	// Figure out the manifest URL based on region
	// Reference: https://docs.aws.amazon.com/eks/latest/userguide/update-cluster.html
	baseURL, err := getBaseURLForVPCCNIManifest(vpcCNIVersion)
	if err != nil {
		return err
	}
	if strings.HasPrefix(region, "cn-") {
		manifestPath = baseURL + "aws-k8s-cni-cn.yaml"
	} else if region == "us-gov-east-1" {
		manifestPath = baseURL + "aws-k8s-cni-us-gov-east-1.yaml"
	} else if region == "us-gov-west-1" {
		manifestPath = baseURL + "aws-k8s-cni-us-gov-west-1.yaml"
	} else if region == "us-west-2" {
		manifestPath = baseURL + "aws-k8s-cni.yaml"
	} else {
		// This is technically the same manifest as us-west-2, but we need to replace references to us-west-2 with the
		// appropriate region, so we need to first download the manifest to a temporary dir and update the region before
		// applying.
		workingDir, err := ioutil.TempDir("", "kubergrunt-sync")
		if err != nil {
			return err
		}
		defer os.RemoveAll(workingDir)
		manifestPath = filepath.Join(workingDir, "aws-k8s-cni.yaml")

		manifestURL := baseURL + "aws-k8s-cni.yaml"
		if err := downloadVPCCNIManifestAndUpdateRegion(manifestURL, manifestPath, region); err != nil {
			return err
		}
	}
	return kubectl.RunKubectl(kubectlOptions, "apply", "-f", manifestPath)
}

// downloadVPCCNIManifestAndUpdateRegion will download the VPC CNI Kubernetes manifest at the given URL, update the
// region, and save it the provided path. The region is always us-west-2 in the manifest (see
// https://docs.aws.amazon.com/eks/latest/userguide/update-cluster.html)
func downloadVPCCNIManifestAndUpdateRegion(url string, fpath string, region string) error {
	out, err := os.Create(fpath)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer resp.Body.Close()

	// As we stream the body contents, update any references to the region. We use bufio.Scanner so that we read and
	// write line by line, as reading by bytes risks reading a part of the region.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if _, err := fmt.Fprintln(out, strings.ReplaceAll(line, "us-west-2", region)); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}
