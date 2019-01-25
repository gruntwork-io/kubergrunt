package main

import (
	"fmt"
)

// InvalidServiceAccountInfo error is returned when the encoded service account is not encoded correctly.
type InvalidServiceAccountInfo struct {
	EncodedServiceAccount string
}

func (err InvalidServiceAccountInfo) Error() string {
	return fmt.Sprintf("Invalid encoding for ServiceAccount string %s. Expected NAMESPACE/NAME.", err.EncodedServiceAccount)
}
