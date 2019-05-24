package kubectl

import (
	"github.com/gruntwork-io/gruntwork-cli/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrepareTillerRole will construct a new Role struct with the provided
// metadata. The role can later be used to add rules.
func PrepareRole(
	namespace string,
	name string,
	labels map[string]string,
	annotations map[string]string,
	rules []rbacv1.PolicyRule,
) *rbacv1.Role {
	// Cannot use a struct literal due to promoted fields from the ObjectMeta
	newRole := rbacv1.Role{}
	newRole.Name = name
	newRole.Namespace = namespace
	newRole.Labels = labels
	newRole.Annotations = annotations
	newRole.Rules = rules
	return &newRole
}

// CreateRole will create the provided role on the Kubernetes cluster.
func CreateRole(options *KubectlOptions, newRole *rbacv1.Role) error {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return err
	}

	_, err = client.RbacV1().Roles(newRole.Namespace).Create(newRole)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

// GetRole will get an RBAC role by name in the provided namespace
func GetRole(options *KubectlOptions, namespace string, name string) (*rbacv1.Role, error) {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return nil, err
	}

	role, err := client.RbacV1().Roles(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return role, nil
}

// ListRole will list all roles that match the provided filters in the provided namespace
func ListRoles(options *KubectlOptions, namespace string, filters metav1.ListOptions) ([]rbacv1.Role, error) {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return nil, err
	}

	resp, err := client.RbacV1().Roles(namespace).List(filters)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return resp.Items, nil
}

// DeleteRole will delete the role in the provided namespace that has the provided name.
func DeleteRole(options *KubectlOptions, namespace string, name string) error {
	client, err := GetKubernetesClientFromOptions(options)
	if err != nil {
		return err
	}

	err = client.RbacV1().Roles(namespace).Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}
