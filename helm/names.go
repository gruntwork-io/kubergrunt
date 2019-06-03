package helm

import (
	"crypto/md5"
	"fmt"
	"regexp"
)

const (
	NamespaceLabel       = "gruntwork.io/tiller-namespace"
	CredentialsLabel     = "gruntwork.io/tiller-credentials"
	CredentialsTypeLabel = "gruntwork.io/tiller-credentials-type"
	EntityIDLabel        = "gruntwork.io/tiller-entity-id"
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

func getTillerClientCertSecretLabels(entityID string, namespace string) map[string]string {
	return map[string]string{
		NamespaceLabel:       namespace,
		CredentialsLabel:     "true",
		CredentialsTypeLabel: "client",
		EntityIDLabel:        sanitizeLabelValues(entityID),
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

func getTillerAccessRoleName(entityID string, namespace string) string {
	return fmt.Sprintf("%s-%s-tiller-access", entityID, namespace)
}

func getTillerAccessRoleBindingName(entityID string, roleName string) string {
	return fmt.Sprintf("%s-%s-binding", entityID, roleName)
}

// sanitizeLabelValues will sanitize the provided string so that it can be used as a kubernetes label value. Kubernetes
// labels have the following restrictions:
// - Must be 63 characters or less
// - Begin and end with an alphanumeric character ([a-zA-Z0-9])
// - Only contain dashes (-), underscores (_), dots (.), and alphanumeric characters ([a-zA-Z0-9])
// This sanitization will handle the unsupported characters piece ONLY. Specifically, this will take all unsupported
// characters and replace them with dashes (-). E.g if you have the value "foo@bar", this will be converted to
// "foo-bar".
func sanitizeLabelValues(value string) string {
	re := regexp.MustCompile(`[^0-9A-Za-z-_.]`)
	return re.ReplaceAllString(value, "-")
}

func getTillerRoleLabels(entityID string, namespace string) map[string]string {
	// Here we only sanitize the role name because namespace names are already constrained with the same restrictions as
	// node labels, and thus you can't create or reference a namespace that is not a valid label.
	return map[string]string{
		NamespaceLabel: namespace,
		EntityIDLabel:  sanitizeLabelValues(entityID),
	}
}

func getTillerRoleBindingLabels(entityID string, namespace string) map[string]string {
	// Here we only sanitize the role binding name because namespace names are already constrained with the same
	// restrictions as node labels, and thus you can't create or reference a namespace that is not a valid label.
	return map[string]string{
		NamespaceLabel: namespace,
		EntityIDLabel:  sanitizeLabelValues(entityID),
	}
}
