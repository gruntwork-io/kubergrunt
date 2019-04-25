package eks

import (
	"io/ioutil"
	"math"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/gruntwork-io/gruntwork-cli/errors"

	"github.com/gruntwork-io/kubergrunt/logging"
)

// GetClusterByArn returns the EKS Cluster object that corresponds to the given ARN.
func GetClusterByArn(eksClusterArn string) (*eks.Cluster, error) {
	logger := logging.GetProjectLogger()
	logger.Infof("Retrieving details for EKS cluster %s", eksClusterArn)

	region, err := GetRegionFromArn(eksClusterArn)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	logger.Infof("Detected cluster deployed in region %s", region)

	client, err := NewEksClient(region)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	eksClusterName, err := GetClusterNameFromArn(eksClusterArn)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	describeClusterOutput, err := client.DescribeCluster(&eks.DescribeClusterInput{Name: aws.String(eksClusterName)})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	logger.Infof("Successfully retrieved EKS cluster details")

	return describeClusterOutput.Cluster, nil
}

// VerifyCluster verifies that the cluster exists, and that the Kubernetes api server is up and accepting traffic.
// If waitForCluster is true, this command will wait for each stage to reach the true state.
func VerifyCluster(
	eksClusterArn string,
	waitForCluster bool,
	waitMaxRetries int,
	waitSleepBetweenRetries time.Duration,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Checking if EKS cluster %s exists", eksClusterArn)

	if waitForCluster && waitMaxRetries == 0 {
		// Default is 5 minutes / duration
		waitMaxRetries = int(math.Trunc(300 / waitSleepBetweenRetries.Seconds()))
	}

	clusterInfo, err := GetClusterByArn(eksClusterArn)
	if err == nil && !clusterIsActive(clusterInfo) {
		err = EKSClusterNotReady{eksClusterArn}
	}
	if err != nil {
		logger.Errorf("EKS cluster %s is not active yet", eksClusterArn)
		if !waitForCluster {
			logger.Errorf("Did not specify wait. Aborting...")
			return err
		}
		err = waitForClusterActive(eksClusterArn, waitMaxRetries, waitSleepBetweenRetries)
		if err != nil {
			return err
		}
	}

	logger.Infof("Verified EKS cluster %s is in active state.", eksClusterArn)

	logger.Infof("Checking EKS cluster %s Kubernetes API endpoint.", eksClusterArn)
	available := checkKubernetesApiServer(eksClusterArn)
	if !available && !waitForCluster {
		logger.Errorf("Kubernetes api server is not ready yet")
		logger.Errorf("Did not specify wait. Aborting...")
		return errors.WithStackTrace(EKSClusterNotReady{eksClusterArn})
	}
	if !available {
		err = waitForKubernetesApiServer(eksClusterArn, waitMaxRetries, waitSleepBetweenRetries)
		if err != nil {
			return err
		}
	}
	logger.Infof("Verified EKS cluster %s Kubernetes API endpoint is available.", eksClusterArn)

	return nil
}

func clusterIsActive(clusterInfo *eks.Cluster) bool {
	return clusterInfo != nil && aws.StringValue(clusterInfo.Status) == "ACTIVE"
}

// waitForClusterActive continuously queries the AWS API until the cluster reaches the ACTIVE state.
func waitForClusterActive(eksClusterArn string, maxRetries int, sleepBetweenRetries time.Duration) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Waiting for cluster %s to reach active state.", eksClusterArn)
	for i := 0; i < maxRetries; i++ {
		logger.Info("Checking EKS cluster info")
		clusterInfo, err := GetClusterByArn(eksClusterArn)
		// We do nothing with the error other than log, because it could mean the cluster hasn't been created yet.
		if err != nil {
			logger.Warnf("Error retrieving cluster info %s", err)
		}
		if clusterIsActive(clusterInfo) {
			logger.Infof("EKS cluster %s is active", eksClusterArn)
			return nil
		}
		logger.Warnf("EKS cluster %s is not active yet", eksClusterArn)
		logger.Infof("Waiting for %s...", sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}
	return errors.WithStackTrace(EKSClusterReadyTimeoutError{eksClusterArn})
}

// checkKubernetesApiServer checks if the api server is up and accepting traffic.
func checkKubernetesApiServer(eksClusterArn string) bool {
	logger := logging.GetProjectLogger()
	logger.Info("Checking EKS cluster info")
	clusterInfo, err := GetClusterByArn(eksClusterArn)
	if err != nil {
		logger.Warnf("Error retrieving cluster info %s", err)
		logger.Warnf("Marking api server as not ready")
		return false
	}
	endpoint := aws.StringValue(clusterInfo.Endpoint)
	if endpoint == "" {
		logger.Warnf("Api server endpoint not available")
		logger.Warnf("Marking api server as not ready")
		return false
	}

	certificate := aws.StringValue(clusterInfo.CertificateAuthority.Data)
	client, err := loadHttpClientWithCA(certificate)
	if err != nil {
		logger.Errorf("Error loading certificate for EKS cluster %s endpoint: %s", eksClusterArn, err)
		logger.Warnf("Marking api server as not ready")
		return false
	}
	resp, err := client.Head(endpoint)
	if err != nil {
		logger.Warnf("Error retrieiving info from endpoint: %s", err)
		logger.Warnf("Marking api server as not ready")
		return false
	}
	// We look for 200 or 403 response. Both indicate the API server is up.
	// A 403 response will be returned from EKS in most situations because we are not going through the auth workflow
	// here to access the API (to keep things simple), and anonymous access is disabled on the cluster (for security
	// reasons).
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logger.Errorf("Error reading response body: %s", err)
			return false
		}
		bodyString := string(bodyBytes)
		logger.Warnf(
			"Received unexpected status code from endpoint: status code - %d; body - %s",
			resp.StatusCode,
			bodyString,
		)
		logger.Warnf("Marking api server as not ready")
		return false
	}

	return true
}

// waitForKubernetesApiServer continuously checks if the api server is up until timing out.
func waitForKubernetesApiServer(eksClusterArn string, maxRetries int, sleepBetweenRetries time.Duration) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Waiting for cluster %s Kubernetes api server to accept traffic.", eksClusterArn)
	for i := 0; i < maxRetries; i++ {
		logger.Info("Checking EKS cluster info")
		available := checkKubernetesApiServer(eksClusterArn)
		if available {
			logger.Infof("EKS cluster %s Kubernetes api server is active", eksClusterArn)
			return nil
		}
		logger.Warnf("EKS cluster %s Kubernetes api server is not active yet", eksClusterArn)
		logger.Infof("Waiting for %s...", sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}
	return errors.WithStackTrace(EKSClusterReadyTimeoutError{eksClusterArn})
}

// NewEksClient creates an EKS client.
func NewEksClient(region string) (*eks.EKS, error) {
	sess, err := NewAuthenticatedSession(region)
	if err != nil {
		return nil, err
	}
	return eks.New(sess), nil
}
