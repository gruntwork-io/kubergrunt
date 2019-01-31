package helm

import (
	"fmt"
)

// UnknownRBACEntityType error is returned when the RBAC entity type is something unexpected
type UnknownRBACEntityType struct {
	RBACEntityType string
}

func (err UnknownRBACEntityType) Error() string {
	return fmt.Sprintf("%s is an unknown RBAC entity type", err.RBACEntityType)
}

// InvalidServiceAccountInfo error is returned when the encoded service account is not encoded correctly.
type InvalidServiceAccountInfo struct {
	EncodedServiceAccount string
}

func (err InvalidServiceAccountInfo) Error() string {
	return fmt.Sprintf("Invalid encoding for ServiceAccount string %s. Expected NAMESPACE/NAME.", err.EncodedServiceAccount)
}

// HelmValidationError is returned when a command validation fails.
type HelmValidationError struct {
	Message string
}

func (err HelmValidationError) Error() string {
	return err.Message
}

// MultiHelmError is returned when there are multiple errors in a helm action.
type MultiHelmError struct {
	Action string
	Errors []error
}

func (err MultiHelmError) Error() string {
	base := fmt.Sprintf("Multiple errors caught while performing helm action %s:\n", err.Action)
	for _, subErr := range err.Errors {
		subErrMessage := fmt.Sprintf("%s", subErr)
		base = base + subErrMessage + "\n"
	}
	return base
}

func (err MultiHelmError) AddError(newErr error) {
	err.Errors = append(err.Errors, newErr)
}

func (err MultiHelmError) IsEmpty() bool {
	return len(err.Errors) == 0
}
