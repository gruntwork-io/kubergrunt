package helm

import (
	"fmt"
)

func getTillerClientCertSecretName(entityName string) string {
	return fmt.Sprintf("tiller-client-%s-certs", entityName)
}

func getTillerCACertSecretName(tillerNamespace string) string {
	return fmt.Sprintf("%s-namespace-tiller-ca-certs", tillerNamespace)
}
