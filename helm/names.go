package helm

import (
	"crypto/md5"
	"fmt"
)

// NOTE: RBAC has relaxed constraints for names compared to resource names. Specifically, RBAC names allow many more
// special characters compared to resources (only '_' and '.' is supported). Here we overcome this by using a md5 hash
// of the entity name.
func getTillerClientCertSecretName(entityName string) string {
	return fmt.Sprintf("tiller-client-%s-certs", md5HashString(entityName))
}

func getTillerCACertSecretName(tillerNamespace string) string {
	return fmt.Sprintf("%s-namespace-tiller-ca-certs", tillerNamespace)
}

func md5HashString(input string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(input)))
}
