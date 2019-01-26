package helm

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

// ConfigureClient will configure the local helm client to be able to communicate with the Tiller server installed in
// the provided Tiller namespace. Note that this supports the notion where Tiller is deployed in a different namespace
// from where resources should go. This is to address the risk where access to the tiller-secret will grant admin access
// by using the tiller server TLS certs.
func ConfigureClient(
	kubectlOptions *kubectl.KubectlOptions,
	helmHome string,
	tillerNamespace string,
	resourceNamespace string,
	setKubectlNamespace bool,
	rbacEntityName string,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Setting up local helm client to access Tiller server deployed in namespace %s.", tillerNamespace)

	logger.Info("Checking if authorized to access specified Tiller server.")
	// Check for
	// - Access to Tiller pod. If unavailable, mention they need to be granted access, pod should be deployed, or change
	//   namespace.
	if err := verifyAccessToTillerPod(kubectlOptions, tillerNamespace); err != nil {
		logger.Errorf("You do not have permissions to access the Tiller endpoint in namespace %s, or Tiller does not exist.", tillerNamespace)
		return err
	}
	// - Access to TLS certs. If unavailable, mention they need to be granted access.
	secret, err := getClientCertsSecret(kubectlOptions, tillerNamespace, rbacEntityName)
	if err != nil {
		logger.Errorf("You do not have permissions to access the client certs for Tiller deployed in namespace %s, or they do not exist.", tillerNamespace)
		return err
	}
	logger.Info("Confirmed authorized to access specified Tiller server.")

	logger.Info("Downloading TLS certificates to access specified Tiller server.")
	if err := downloadTLSCertificatesToHelmHome(helmHome, secret); err != nil {
		return err
	}
	logger.Info("Successfully downloaded TLS certificates.")

	logger.Info("Generating environment file to setup helm client.")
	// TODO
	logger.Info("Successfully generated environment file.")

	if setKubectlNamespace {
		logger.Info("Requested to set default kubectl namespace.")
		// TODO
		logger.Infof("Updated context %s to use namespace %s as default.", kubectlOptions.ContextName, resourceNamespace)
	}

	logger.Infof("Successfully set up local helm client to access Tiller server deployed in namespace %s. Be sure to source the environment file (%s/env) before using the helm client.", tillerNamespace, helmHome)
	return nil
}

// verifyAccessToTillerPod checks if the authenticated client has access to the Tiller pod endpoint.
func verifyAccessToTillerPod(kubectlOptions *kubectl.KubectlOptions, tillerNamespace string) error {
	filters := metav1.ListOptions{LabelSelector: "app=helm,name=tiller"}
	pods, err := kubectl.ListPods(kubectlOptions, tillerNamespace, filters)
	if err != nil {
		return err
	}
	if len(pods) == 0 {
		msg := fmt.Sprintf("Could not find Tiller pod in namespace %s.", tillerNamespace)
		return errors.WithStackTrace(HelmValidationError{msg})
	}
	return nil
}

// getClientCertsSecret gets the Kubernetes Secret resource corresponding to be a client certificate key pair for
// authenticating with the Tiller instance deployed in the provided namespace.
func getClientCertsSecret(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	rbacEntityName string,
) (*corev1.Secret, error) {
	clientSecretName := getTillerClientCertSecretName(rbacEntityName)
	return kubectl.GetSecret(kubectlOptions, tillerNamespace, clientSecretName)
}

func downloadTLSCertificatesToHelmHome(helmHome string, secret *corev1.Secret) error {
	decodedCACertData := secret.Data["ca.crt"]
	if err := ioutil.WriteFile(filepath.Join(helmHome, "ca.pem"), decodedCACertData, 0644); err != nil {
		return errors.WithStackTrace(err)
	}

	decodedClientPrivateKeyData := secret.Data["client.pem"]
	if err := ioutil.WriteFile(filepath.Join(helmHome, "key.pem"), decodedClientPrivateKeyData, 0644); err != nil {
		return errors.WithStackTrace(err)
	}

	decodedClientCertData := secret.Data["client.crt"]
	if err := ioutil.WriteFile(filepath.Join(helmHome, "cert.pem"), decodedClientCertData, 0644); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
