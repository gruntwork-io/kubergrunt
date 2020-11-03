package eks

import (
	"fmt"
	"strings"
)

// EKSClusterNotReady is returned when the EKS cluster is detected to not be in the ready state
type EKSClusterNotReady struct {
	eksClusterArn string
}

func (err EKSClusterNotReady) Error() string {
	return fmt.Sprintf("EKS cluster %s is not ready", err.eksClusterArn)
}

// EKSClusterReadyTimeoutError is returned when we time out waiting for an EKS cluster to be ready.
type EKSClusterReadyTimeoutError struct {
	eksClusterArn string
}

func (err EKSClusterReadyTimeoutError) Error() string {
	return fmt.Sprintf(
		"Timed out waiting for EKS cluster %s to reach ready state.",
		err.eksClusterArn,
	)
}

// CouldNotMeetASGCapacityError represents an error related to waiting for ASG to reach desired capacity
type CouldNotMeetASGCapacityError struct {
	asgName string
	message string
}

func (err CouldNotMeetASGCapacityError) Error() string {
	return fmt.Sprintf(
		"Could not reach desired capacity of ASG %s: %s",
		err.asgName,
		err.message,
	)
}

func NewCouldNotMeetASGCapacityError(asgName string, message string) CouldNotMeetASGCapacityError {
	return CouldNotMeetASGCapacityError{asgName, message}
}

// MultipleTerminateInstanceErrors represents multiple errors found while terminating instances
type MultipleTerminateInstanceErrors struct {
	errors []error
}

func (err MultipleTerminateInstanceErrors) Error() string {
	messages := []string{
		fmt.Sprintf("%d errors found while terminating instances:", len(err.errors)),
	}

	for _, individualErr := range err.errors {
		messages = append(messages, individualErr.Error())
	}
	return strings.Join(messages, "\n")
}

func (err MultipleTerminateInstanceErrors) AddError(newErr error) {
	err.errors = append(err.errors, newErr)
}

func (err MultipleTerminateInstanceErrors) IsEmpty() bool {
	return len(err.errors) == 0
}

func NewMultipleTerminateInstanceErrors() MultipleTerminateInstanceErrors {
	return MultipleTerminateInstanceErrors{[]error{}}
}

// MultipleLookupErrors represents multiple errors found while looking up a resource
type MultipleLookupErrors struct {
	errors []error
}

func (err MultipleLookupErrors) Error() string {
	messages := []string{
		fmt.Sprintf("%d errors found during lookup:", len(err.errors)),
	}

	for _, individualErr := range err.errors {
		messages = append(messages, individualErr.Error())
	}
	return strings.Join(messages, "\n")
}

func (err MultipleLookupErrors) AddError(newErr error) {
	err.errors = append(err.errors, newErr)
}

func (err MultipleLookupErrors) IsEmpty() bool {
	return len(err.errors) == 0
}

func NewMultipleLookupErrors() MultipleLookupErrors {
	return MultipleLookupErrors{[]error{}}
}

// LookupError represents an error related to looking up data on an object.
type LookupError struct {
	objectProperty string
	objectType     string
	objectId       string
}

func (err LookupError) Error() string {
	return fmt.Sprintf("Failed to look up %s for %s with id %s.", err.objectProperty, err.objectType, err.objectId)
}

// NewLookupError constructs a new LookupError object that can be used to return an error related to a look up error.
func NewLookupError(objectType string, objectId string, objectProperty string) LookupError {
	return LookupError{objectProperty: objectProperty, objectType: objectType, objectId: objectId}
}

// NoPeerCertificatesError is returned when we couldn't find any TLS peer certificates for the provided URL.
type NoPeerCertificatesError struct {
	URL string
}

func (err NoPeerCertificatesError) Error() string {
	return fmt.Sprintf("Could not find any peer certificates for URL %s", err.URL)
}

// UnsupportedEKSVersion is returned when the Kubernetes version of the EKS cluster is not supported.
type UnsupportedEKSVersion struct {
	version string
}

func (err UnsupportedEKSVersion) Error() string {
	return fmt.Sprintf("%s is not a supported version for kubergrunt eks upgrade. Please contact support@gruntwork.io for more info.", err.version)
}

// CoreComponentUnexpectedConfigurationErr error is returned when the EKS core components are in an unexpected
// configuration, such as a different number of containers.
type CoreComponentUnexpectedConfigurationErr struct {
	component string
	reason    string
}

func (err CoreComponentUnexpectedConfigurationErr) Error() string {
	return fmt.Sprintf("Core component %s is in unexpected configuration: %s", err.component, err.reason)
}

// NetworkInterfaceDetachedTimeoutError is returned when we time out waiting for a network interface to be detached.
type NetworkInterfaceDetachedTimeoutError struct {
	networkInterfaceId string
}

func (err NetworkInterfaceDetachedTimeoutError) Error() string {
	return fmt.Sprintf(
		"Timed out waiting for network interface %s to reach detached state.",
		err.networkInterfaceId,
	)
}
