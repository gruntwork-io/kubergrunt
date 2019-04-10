package kubectl

import (
	"io/ioutil"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrepareSecret will construct a new Secret struct with the provided metadata. This can then be used to append data to
// it, either from a file (using AddToSecretFromFile) or raw data (using AddToSecretFromData).
func PrepareSecret(
	namespace string,
	name string,
	labels map[string]string,
	annotations map[string]string,
) *corev1.Secret {
	newSecret := corev1.Secret{}
	newSecret.Name = name
	newSecret.Namespace = namespace
	newSecret.Labels = labels
	newSecret.Annotations = annotations
	newSecret.Data = map[string][]byte{}
	return &newSecret
}

// AddToSecretFromFile will add data to the secret from a file, attached using the provided key.
func AddToSecretFromFile(secret *corev1.Secret, key string, path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	secret.Data[key] = data
	return nil
}

// AddToSecretFromData will add data to the secret at the provided key.
func AddToSecretFromData(secret *corev1.Secret, key string, rawData []byte) {
	secret.Data[key] = rawData
}

// CreateSecret will create the provided secret on the Kubernetes cluster.
func CreateSecret(options *KubectlOptions, newSecret *corev1.Secret) error {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return err
	}

	_, err = client.CoreV1().Secrets(newSecret.Namespace).Create(newSecret)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// GetSecret will get a Kubernetes secret by name in the provided namespace.
func GetSecret(options *KubectlOptions, namespace string, name string) (*corev1.Secret, error) {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return nil, err
	}

	secret, err := client.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return secret, nil
}

// ListSecrets will list all the secrets that match the provided filters in the provided namespace.
func ListSecrets(options *KubectlOptions, namespace string, filters metav1.ListOptions) ([]corev1.Secret, error) {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return nil, err
	}

	resp, err := client.CoreV1().Secrets(namespace).List(filters)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return resp.Items, nil
}

// DeleteSecret will delete the secret in the provided namespace that has the provided name.
func DeleteSecret(options *KubectlOptions, namespace string, secretName string) error {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return err
	}

	err = client.CoreV1().Secrets(namespace).Delete(secretName, &metav1.DeleteOptions{})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}
