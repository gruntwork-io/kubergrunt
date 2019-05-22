package helm

import (
	"crypto/md5"
	"fmt"
)

const (
	NamespaceLabel       = "gruntwork.io/tiller-namespace"
	CredentialsLabel     = "gruntwork.io/tiller-credentials"
	CredentialsTypeLabel = "gruntwork.io/tiller-credentials-type"
	CredentialsNameLabel = "gruntwork.io/tiller-credentials-name"
	RoleNameLabel        = "gruntwork.io/tiller-access-role-name"
	RoleBindingNameLabel = "gruntwork.io/tiller-access-rolebinding-name"
)

// NOTE: RBAC has relaxed constraints for names compared to resource names. Specifically, RBAC names allow many more
// special characters compared to resources (only '_' and '.' is supported). Here we overcome this by using a md5 hash
// of the entity name.
func getTillerClientCertSecretName(entityName string) string {
	return fmt.Sprintf("tiller-client-%s-certs", md5HashString(entityName))
}

func md5HashString(input string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(input)))
}

func getTillerClientCertSecretLabels(entityID, namespace string) map[string]string {
	return map[string]string{
		NamespaceLabel:       namespace,
		CredentialsLabel:     "true",
		CredentialsTypeLabel: "client",
		CredentialsNameLabel: getTillerClientCertSecretName(entityID),
	}
}

func getTillerCACertSecretName(tillerNamespace string) string {
	return fmt.Sprintf("%s-namespace-tiller-ca-certs", tillerNamespace)
}

func getTillerCACertSecretLabels(namespace string) map[string]string {
	return map[string]string{
		NamespaceLabel:       namespace,
		CredentialsLabel:     "true",
		CredentialsTypeLabel: "ca",
	}
}

func getTillerAccessRoleName(entityID, namespace string) string {
	return fmt.Sprintf("%s-%s-tiller-access", entityID, namespace)
}

func getTillerAccessRoleBindingName(entityID, roleName string) string {
	return fmt.Sprintf("%s-%s-binding", entityID, roleName)
}

func getTillerRoleLabels(entityID, namespace string) map[string]string {
	return map[string]string{
		NamespaceLabel: namespace,
		RoleNameLabel:  getTillerAccessRoleName(entityID, namespace),
	}
}

func getTillerRoleBindingLabels(entityID, namespace string) map[string]string {
	return map[string]string{
		NamespaceLabel:       namespace,
		RoleBindingNameLabel: getTillerAccessRoleBindingName(entityID, namespace),
	}
}
