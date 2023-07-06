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
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/blang/semver/v4"
	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/gruntwork-io/kubergrunt/commonerrors"
	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/kubergrunt/jsonpatch"
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

const (
	kubeProxyRepoPath = "eks/kube-proxy"
	coreDNSRepoPath   = "eks/coredns"

	// Largest eksbuild tag we will try looking for.
	maxEKSBuild = 10
)

var (
	// NOTE: Ensure that there is an entry for each supported version in the following tables.
	supportedVersions = []string{"1.27", "1.26", "1.25", "1.24", "1.23"}

	// Reference: https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html
	coreDNSVersionLookupTable = map[string]string{
		"1.27": "1.10.1-eksbuild",
		"1.26": "1.9.3-eksbuild",
		"1.25": "1.9.3-eksbuild",
		"1.24": "1.8.7-eksbuild",
		"1.23": "1.8.7-eksbuild",
	}

	// Reference: https://docs.aws.amazon.com/eks/latest/userguide/managing-kube-proxy.html#updating-kube-proxy-add-on
	kubeProxyVersionLookupTable = map[string]string{
		"1.27": "1.27.1-minimal-eksbuild",
		"1.26": "1.26.2-minimal-eksbuild",
		"1.25": "1.25.6-minimal-eksbuild",
		"1.24": "1.24.7-minimal-eksbuild",
		"1.23": "1.23.8-minimal-eksbuild",
	}

	// Reference: https://docs.aws.amazon.com/eks/latest/userguide/managing-vpc-cni.html
	amazonVPCCNIVersionLookupTable = map[string]string{
		"1.27": "1.13.2",
		"1.26": "1.12.6",
		"1.25": "1.12.2",
		"1.24": "1.11.4",
		"1.23": "1.11.4",
	}

	defaultContainerImageAccount = "602401143452"
	// Reference: https://docs.aws.amazon.com/eks/latest/userguide/add-ons-images.html
	containerImageAccountLookupTable = map[string]string{
		"af-south-1":     "877085696533",
		"ap-east-1":      "800184023465",
		"cn-north-1":     "918309763551",
		"cn-northwest-1": "961992271922",
		"eu-south-1":     "590381155156",
		"me-south-1":     "558608220178",
		"us-gov-east-1":  "151742754352",
		"us-gov-west-1":  "013241004608",
	}
)

const (
	componentNamespace        = "kube-system"
	kubeProxyDaemonSetName    = "kube-proxy"
	corednsDeploymentName     = "coredns"
	corednsClusterRoleName    = "system:coredns"
	corednsConfigMapName      = "coredns"
	corednsConfigMapConfigKey = "Corefile"

	endpointslicesAPIGroup = "discovery.k8s.io"
	endpointslicesResource = "endpointslices"
)

// SkipComponentsConfig represents the components that should be skipped in the sync command.
type SkipComponentsConfig struct {
	KubeProxy bool
	CoreDNS   bool
	VPCCNI    bool
}

type componentVersions struct {
	kubeProxy string
	coreDNS   string
	vpcCNI    string
}

// SyncClusterComponents will perform the steps described in
// https://docs.aws.amazon.com/eks/latest/userguide/update-cluster.html
// There are three core applications on an EKS cluster:
//
//   - kube-proxy
//   - coredns
//   - VPC CNI Plugin
//
// Each of these is managed in Kubernetes as DaemonSet, Deployment, and DaemonSet respectively. This command will use
// the k8s API and kubectl command under the hood to patch the manifests to deploy the expected version based on what
// the current Kubernetes version is of the cluster. As such, this command should be run every time the Kubernetes
// version is updated on the EKS cluster.
func SyncClusterComponents(
	eksClusterArn string,
	shouldWait bool,
	waitTimeout string,
	skipConfig SkipComponentsConfig,
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

	awsRegion, err := eksawshelper.GetRegionFromArn(eksClusterArn)
	if err != nil {
		return err
	}

	dockerToken, err := eksawshelper.GetDockerLoginToken(awsRegion)
	if err != nil {
		return err
	}

	repoDomain := getRepoDomain(awsRegion)
	kubeProxyVersion, err := findLatestEKSBuild(dockerToken, repoDomain, kubeProxyRepoPath, kubeProxyVersionLookupTable[k8sVersion])
	if err != nil {
		return err
	}

	coreDNSVersion, err := findLatestEKSBuild(dockerToken, repoDomain, coreDNSRepoPath, coreDNSVersionLookupTable[k8sVersion])
	if err != nil {
		return err
	}

	amznVPCCNIVersion := amazonVPCCNIVersionLookupTable[k8sVersion]

	logger.Info("Syncing Kubernetes Applications to:")
	if !skipConfig.KubeProxy {
		logger.Infof("\tkube-proxy:\t%s", kubeProxyVersion)
	}
	if !skipConfig.CoreDNS {
		logger.Infof("\tcoredns:\t%s", coreDNSVersion)
	}
	if !skipConfig.VPCCNI {
		logger.Infof("\tVPC CNI Plugin:\t%s", amznVPCCNIVersion)
	}

	kubectlOptions := &kubectl.KubectlOptions{EKSClusterArn: eksClusterArn}
	clientset, err := kubectl.GetKubernetesClientFromOptions(kubectlOptions)
	if err != nil {
		return err
	}

	if skipConfig.KubeProxy {
		logger.Info("Skipping kube-proxy sync.")
	} else {
		if err := upgradeKubeProxy(kubectlOptions, clientset, awsRegion, kubeProxyVersion, shouldWait, waitTimeout); err != nil {
			return err
		}
	}

	if skipConfig.CoreDNS {
		logger.Info("Skipping coredns sync.")
	} else {
		if err := upgradeCoreDNS(kubectlOptions, clientset, awsRegion, coreDNSVersion, shouldWait, waitTimeout); err != nil {
			return err
		}
	}

	if skipConfig.VPCCNI {
		logger.Info("Skipping aws-vpc-cni.")
	} else {
		if err := updateVPCCNI(kubectlOptions, awsRegion, amznVPCCNIVersion); err != nil {
			return err
		}
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

	targetImage := fmt.Sprintf("%s/%s:v%s", getRepoDomain(awsRegion), kubeProxyRepoPath, kubeProxyVersion)
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

	logger.Info("Confirming compatibility of coredns configuration with latest version.")
	// Need to check config for backwards incompatibility if updating to version >= 1.7.0. The keyword `upstream` was
	// removed in 1.7 series of coredns, but is used in earlier versions.
	compareVal170, err := semverStringCompare(coreDNSVersion, "1.7.0-eksbuild.1")
	if err != nil {
		return err
	}
	// Looking for 1 or 0 here, since we want to know when coreDNSVersion is >= 1.7.0
	if compareVal170 >= 0 {
		if err := updateCorednsConfigMapFor170Compatibility(clientset); err != nil {
			return err
		}
	} else {
		logger.Info("Configuration for coredns is up to date. Skipping configuration reformat.")
	}

	logger.Info("Confirming compatibility of coredns permissions with latest version.")
	// Need to check permissions compatibility if updating to version >= 1.8.3. Starting with 1.8.3, coredns requires
	// permissions to list and watch endpoint slices.
	compareVal183, err := semverStringCompare(coreDNSVersion, "1.8.3-eksbuild.1")
	if err != nil {
		return err
	}
	if compareVal183 >= 0 {
		if err := updateCorednsPermissionsFor183Compatibility(clientset); err != nil {
			return err
		}
	} else {
		logger.Info("ClusterRole permissions for coredns is up to date. Skipping adjusting ClusterRole permissions.")
	}

	targetImage := fmt.Sprintf("%s/%s:v%s", getRepoDomain(awsRegion), coreDNSRepoPath, coreDNSVersion)
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

// updateCorednsConfigMapFor170Compatibility updates the ConfigMap to remove traces of the upstream keyword, which was
// removed starting with 1.7.0.
func updateCorednsConfigMapFor170Compatibility(clientset *kubernetes.Clientset) error {
	logger := logging.GetProjectLogger()
	corednsConfigMap, err := getCorednsConfigMap(clientset)
	if err != nil {
		return err
	}
	if strings.Contains(corednsConfigMap.Data[corednsConfigMapConfigKey], "upstream") {
		logger.Info("Detected old configuration for coredns. Reformatting configuration to latest.")
		if err := removeUpstreamKeywordFromCorednsConfigMap(clientset, corednsConfigMap); err != nil {
			return err
		}
	}
	return nil
}

// updateCorednsPermissionsFor183Compatibility updates the coredns ClusterRole to include permissions that are
// additionally needed starting with 1.8.3.
func updateCorednsPermissionsFor183Compatibility(clientset *kubernetes.Clientset) error {
	logger := logging.GetProjectLogger()

	corednsClusterRole, err := getCorednsClusterRole(clientset)
	if err != nil {
		return err
	}
	// Check if any of the policy rules overlap with list/watch permissions for endpointslices
	hasListEndpointSlicesRule, hasWatchEndpointSlicesRule := hasEndpointSlicesPermissions(corednsClusterRole.Rules)
	if hasListEndpointSlicesRule && hasWatchEndpointSlicesRule {
		// Already have the necessary permissions, so do nothing
		return nil
	}

	logger.Info("coredns ClusterRole does not have enough permissions for 1.8.3. Updating ClusterRole.")

	// Construct new rule that contains the necessary permissions
	newRule := rbacv1.PolicyRule{
		APIGroups: []string{endpointslicesAPIGroup},
		Resources: []string{endpointslicesResource},
	}
	if !hasListEndpointSlicesRule {
		newRule.Verbs = append(newRule.Verbs, "list")
	}
	if !hasWatchEndpointSlicesRule {
		newRule.Verbs = append(newRule.Verbs, "watch")
	}
	corednsClusterRole.Rules = append(corednsClusterRole.Rules, newRule)

	// Now save the updated ClusterRole
	clusterRoleAPI := clientset.RbacV1().ClusterRoles()
	_, err = clusterRoleAPI.Update(context.Background(), corednsClusterRole, metav1.UpdateOptions{})
	return errors.WithStackTrace(err)
}

// hasEndpointSlicesPermissions checks if the given rules contain the rule for providing list and watch permissions to
// endpointslices. Returns a 2-tuple where the first element indicates having list permissions, and the latter indicates
// having watch permissions.
func hasEndpointSlicesPermissions(rules []rbacv1.PolicyRule) (bool, bool) {
	hasListEndpointSlicesRule := false
	hasWatchEndpointSlicesRule := false
	for _, rule := range rules {
		ruleIsForDiscoveryAPI := collections.ListContainsElement(rule.APIGroups, endpointslicesAPIGroup)
		ruleIsForEndpointSlices := collections.ListContainsElement(rule.Resources, endpointslicesResource) || collections.ListContainsElement(rule.Resources, rbacv1.ResourceAll)
		ruleIsForList := collections.ListContainsElement(rule.Verbs, "list") || collections.ListContainsElement(rule.Verbs, rbacv1.VerbAll)
		ruleIsForWatch := collections.ListContainsElement(rule.Verbs, "watch") || collections.ListContainsElement(rule.Verbs, rbacv1.VerbAll)
		if ruleIsForDiscoveryAPI && ruleIsForEndpointSlices && ruleIsForList {
			hasListEndpointSlicesRule = true
		}
		if ruleIsForDiscoveryAPI && ruleIsForEndpointSlices && ruleIsForWatch {
			hasWatchEndpointSlicesRule = true
		}
	}
	return hasListEndpointSlicesRule, hasWatchEndpointSlicesRule
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

// getCorednsConfigMap returns the configmap object containing the coredns configuration for the EKS cluster.
func getCorednsConfigMap(clientset *kubernetes.Clientset) (*corev1.ConfigMap, error) {
	configMapAPI := clientset.CoreV1().ConfigMaps(componentNamespace)
	configMap, err := configMapAPI.Get(context.Background(), corednsConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return configMap, nil
}

// getCorednsClusterRole returns the ClusterRole object for coredns.
func getCorednsClusterRole(clientset *kubernetes.Clientset) (*rbacv1.ClusterRole, error) {
	clusterRoleAPI := clientset.RbacV1().ClusterRoles()
	corednsClusterRole, err := clusterRoleAPI.Get(context.Background(), corednsClusterRoleName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return corednsClusterRole, nil
}

// getBaseURLForVPCCNIManifest returns the base github URL where the manifest for the VPC CNI is located given the
// requested version.
func getBaseURLForVPCCNIManifest(vpcCNIVersion string) (string, error) {
	baseURL := fmt.Sprintf("https://raw.githubusercontent.com/aws/amazon-vpc-cni-k8s/v%s/config/master/", vpcCNIVersion)
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

// semverStringCompare compares two semantic version strings. Returns:
// -1 if v1 < v2
// 0 if v1 == v2
// 1 if v1 > v2
// Note that in semantic versioning, annotations are considered an older version.
// E.g., 1.7.0-eksbuild.1 is considered less than 1.7.0.
func semverStringCompare(v1 string, v2 string) (int, error) {
	parsedV1, err := semver.Make(v1)
	if err != nil {
		return 0, errors.WithStackTrace(err)
	}
	parsedV2, err := semver.Make(v2)
	if err != nil {
		return 0, errors.WithStackTrace(err)
	}
	return parsedV1.Compare(parsedV2), nil
}

// removeUpstreamKeywordFromCorednsConfigMap removes the upstream keyword from the CoreDNS ConfigMap config data and
// saves it on the cluster.
func removeUpstreamKeywordFromCorednsConfigMap(clientset *kubernetes.Clientset, corednsConfigMap *corev1.ConfigMap) error {
	configData := corednsConfigMap.Data[corednsConfigMapConfigKey]

	// Remove the line containing "upstream". Since this can appear in any nested block, we use regex to handle the
	// whitespace during the removal.
	lookForUpstreamRE, err := regexp.Compile(`[\s]+upstream[\t\r\n]+`)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	newConfigData := lookForUpstreamRE.ReplaceAllString(configData, "\n")
	corednsConfigMap.Data[corednsConfigMapConfigKey] = newConfigData

	// Now save the new configmap
	configMapAPI := clientset.CoreV1().ConfigMaps(corednsConfigMap.ObjectMeta.Namespace)
	_, err = configMapAPI.Update(context.Background(), corednsConfigMap, metav1.UpdateOptions{})
	return errors.WithStackTrace(err)
}

// findLatestEKSBuild will continuously query the ECR repo to look for the latest eksbuild version. We do this by
// incrementally checking one tag at a time until we reach a 404, or the maximum trials.
func findLatestEKSBuild(token, repoDomain, repoPath, tagBase string) (string, error) {
	logger := logging.GetProjectLogger()
	logger.Debugf("Looking up latest eksbuild for repo %s/%s", repoDomain, repoPath)

	if !strings.Contains(tagBase, "eksbuild") {
		logger.Debugf("Not an eksbuild for repo %s/%s, returning %s", repoDomain, repoPath, tagBase)
		return tagBase, nil
	}

	var existingTag string
	for i := 0; i < maxEKSBuild; i++ {
		version := fmt.Sprintf("%s.%d", tagBase, i+1)
		query := "v" + version
		logger.Debugf("Trying %s", query)
		tagExists, err := eksawshelper.TagExistsInRepo(token, repoDomain, repoPath, query)
		if err != nil {
			return "", err
		}
		if tagExists {
			logger.Debugf("Found %s", query)
			// Update the latest tag marker
			existingTag = version
		} else {
			logger.Debugf("Not found %s", query)
			logger.Debugf("Returning %s", existingTag)
			// At this point, the last existing tag we encountered is the latest, so we return it.
			return existingTag, nil
		}
	}

	// MAINTAINER'S NOTE: If we ever reach here, this is 100% a bug in kubergrunt. Investigation is needed to resolve
	// this, as it could be either the wrong version is being queried, or the maxEKSBuild count is too small.
	return "", commonerrors.ImpossibleErr("TOO_MANY_EKS_BUILD_TAGS")
}

// getRepoDomain is a conveniency function to construct the ECR docker repo URL domain.
func getRepoDomain(region string) string {
	containerAccountID := defaultContainerImageAccount
	if id, ok := containerImageAccountLookupTable[region]; ok {
		containerAccountID = id
	}
	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", containerAccountID, region)
}
