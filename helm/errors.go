package helm

import (
	"fmt"
)

// HelmHomeIsFileError is returned when the helm home path is a file and not a directory
type HelmHomeIsFileError struct {
	HelmHome string
}

func (err HelmHomeIsFileError) Error() string {
	return fmt.Sprintf("Provided helm home path %s is a file, not a directory", err.HelmHome)
}

// RepoFileIsDirectoryError is returned when the helm repository file path is unexpectedly a directory
type RepoFileIsDirectoryError struct {
	RepoFilePath string
}

func (err RepoFileIsDirectoryError) Error() string {
	return fmt.Sprintf("Helm repository path %s is a directory, not a file", err.RepoFilePath)
}

// RepositoryUnreachableError is returned when the chart repository is unreachable or does not exist.
type RepositoryUnreachableError struct {
	RepositoryURL   string
	UnderlyingError error
}

func (err RepositoryUnreachableError) Error() string {
	return fmt.Sprintf(
		"Helm repository %s is not a valid chart repository or cannot be reached: %s",
		err.RepositoryURL,
		err.UnderlyingError,
	)
}

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

// TillerDeployWaitTimeoutError is returned when deploy times out waiting for Tiller to come up.
type TillerDeployWaitTimeoutError struct {
	Namespace string
}

func (err TillerDeployWaitTimeoutError) Error() string {
	return fmt.Sprintf("Timed out waiting for Tiller deployment in namespace %s", err.Namespace)
}

// TillerPingError is returned when we fail to reach the Tiller pod using the helm client.
type TillerPingError struct {
	Namespace       string
	UnderlyingError error
}

func (err TillerPingError) Error() string {
	return fmt.Sprintf("Failed to ping Tiller in Namespace %s: %s", err.Namespace, err.UnderlyingError)
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

// ResourceDoesNotExistError is returned when the resource that is being deleted is not found.
type ResourceDoesNotExistError struct {
	Resource string
	Name     string
}

func (d *ResourceDoesNotExistError) Error() string {
	return fmt.Sprintf("%s %s does not exist", d.Resource, d.Name)
}
