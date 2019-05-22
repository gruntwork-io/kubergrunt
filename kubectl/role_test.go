package kubectl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	TestRole = "test-role"
)

/*
import (
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
*/

// Test that we can create a role with read permissions on pods
func TestCreateRoleWithTillerRules(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	// Create a namespace so we don't collide with other tests
	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	testRules := []rbacv1.PolicyRule{
		rbacv1.PolicyRule{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{""},
			Resources: []string{"pods"},
		},
	}
	roleName := "tiller-role-test"
	role := PrepareRole(
		namespace,
		roleName,
		map[string]string{},
		map[string]string{},
		testRules,
	)
	require.NoError(t, CreateRole(kubectlOptions, role))

	// Now verify the role was actually created in the cluster
	// We use the terratest role lib instead of the one in kubectl.
	ttKubectlOptions.Namespace = namespace
	role = k8s.GetRole(t, ttKubectlOptions, roleName)
	assert.Equal(t, role.Name, roleName)
}

// Test that we can get an existing role
func TestGetRole(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	configData := createRole(t, ttKubectlOptions, namespace)
	defer k8s.KubectlDeleteFromString(t, ttKubectlOptions, configData)

	testRoleName := fmt.Sprintf("%s-%s", namespace, TestRole)
	role, err := GetRole(kubectlOptions, namespace, testRoleName)
	require.NoError(t, err)
	assert.Equal(t, role.Name, testRoleName)
	assert.Equal(t, role.Namespace, namespace)
}

// Test that we can Delete an existing role
func TestDeleteRole(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	configData := createRole(t, ttKubectlOptions, namespace)

	testRoleName := getTestRoleName(namespace)
	err := DeleteRole(kubectlOptions, namespace, testRoleName)
	if err != nil {
		k8s.KubectlDeleteFromString(t, ttKubectlOptions, configData)
	}
	require.NoError(t, err)

	// Now verify the role was actually deleted in the cluster
	// Use terratest to get the role and expect an error
	ttKubectlOptions.Namespace = namespace
	emptyRole := &rbacv1.Role{}
	role, err := k8s.GetRoleE(t, ttKubectlOptions, testRoleName)
	require.Error(t, err)
	assert.Equal(t, role, emptyRole)
}

// Test that we can create a role with labels, then list it
func TestCreateAndListWithLabel(t *testing.T) {
	t.Parallel()

	ttKubectlOptions, kubectlOptions := getKubectlOptions(t)

	namespace := strings.ToLower(random.UniqueId())
	k8s.CreateNamespace(t, ttKubectlOptions, namespace)
	defer k8s.DeleteNamespace(t, ttKubectlOptions, namespace)

	testRules := []rbacv1.PolicyRule{
		rbacv1.PolicyRule{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{""},
			Resources: []string{"pods"},
		},
	}
	roleName := getTestRoleName(namespace)
	role := PrepareRole(
		namespace,
		roleName,
		getTestLabels(),
		map[string]string{},
		testRules,
	)
	err := CreateRole(kubectlOptions, role)
	require.NoError(t, err)

	labels := LabelsToListOptions(getTestLabels())
	roles, err := ListRoles(kubectlOptions, namespace, labels)
	require.NoError(t, err)
	assert.NotEmpty(t, roles)
	assert.Equal(t, roleName, roles[0].Name)
}

// Utility functions used in the tests in this file
func getTestRoleName(namespace string) string {
	return fmt.Sprintf("%s-%s", namespace, TestRole)
}

func getTestLabels() map[string]string {
	return map[string]string{
		"gruntwork.io/test-key":     "value",
		"gruntwork.io/test-key-two": "value-two",
	}
}

func createRole(t *testing.T, options *k8s.KubectlOptions, namespace string) string {
	configData := fmt.Sprintf(
		EXAMPLE_ROLE_YAML_TEMPLATE,
		getTestRoleName(namespace),
		namespace,
	)
	k8s.KubectlApplyFromString(t, options, configData)
	return configData
}

// func getKubectlOptions(t *testing.T) (*k8s.KubectlOptions, *KubectlOptions) {
// 	ttKubectlOptions := k8s.NewKubectlOptions("", "")
// 	configPath, err := k8s.KubeConfigPathFromHomeDirE()
// 	require.NoError(t, err)
// 	kubectlOptions := &KubectlOptions{ConfigPath: configPath}
// 	return ttKubectlOptions, kubectlOptions
// }

const EXAMPLE_ROLE_YAML_TEMPLATE = `---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s
  namespace: %s
rules:
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - get
  - list
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - get
- apiGroups:
  - ""
  resources:
  - pods/portforward
  verbs:
  - create`
