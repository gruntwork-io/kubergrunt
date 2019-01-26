package helm

import (
	"fmt"
)

func getTillerClientCertSecretName(entityName string) string {
	return fmt.Sprintf("tiller-client-%s-certs", entityName)
}
