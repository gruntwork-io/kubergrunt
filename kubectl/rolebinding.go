package kubectl

import (
	"context"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrepareTillerRoleBinding will construct a new RoleBinding struct with the provided metadata. The role can later
// be used to add rules.
func PrepareRoleBinding(
	namespace string,
	name string,
	labels map[string]string,
	annotations map[string]string,
	subjects []rbacv1.Subject,
	roleRef rbacv1.RoleRef,
) *rbacv1.RoleBinding {
	newRoleBinding := rbacv1.RoleBinding{}
	newRoleBinding.Name = name
	newRoleBinding.Namespace = namespace
	newRoleBinding.Labels = labels
	newRoleBinding.Annotations = annotations
	newRoleBinding.Subjects = subjects
	newRoleBinding.RoleRef = roleRef
	return &newRoleBinding
}

// CreateRoleBinding will create the provided role binding on the Kubernetes cluster.
func CreateRoleBinding(options *KubectlOptions, newRoleBinding *rbacv1.RoleBinding) error {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return err
	}

	_, err = client.RbacV1().RoleBindings(newRoleBinding.Namespace).Create(context.Background(), newRoleBinding, metav1.CreateOptions{})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// GetRoleBinding will get an RBAC role binding by name in the provided namespace
func GetRoleBinding(options *KubectlOptions, namespace string, name string) (*rbacv1.RoleBinding, error) {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return nil, err
	}

	roleBinding, err := client.RbacV1().RoleBindings(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return roleBinding, nil
}

// ListRoleBindings will list all role bindings that match the provided filters in the provided namespace
func ListRoleBindings(options *KubectlOptions, namespace string, filters metav1.ListOptions) ([]rbacv1.RoleBinding, error) {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return nil, err
	}

	resp, err := client.RbacV1().RoleBindings(namespace).List(context.Background(), filters)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return resp.Items, nil
}

// DeleteRole will delete the role in the provided namespace that has the provided name.
func DeleteRoleBinding(options *KubectlOptions, namespace string, name string) error {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return err
	}

	err = client.RbacV1().RoleBindings(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}
