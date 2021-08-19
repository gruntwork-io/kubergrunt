package eksawshelper

import "fmt"

// CredentialsError is an error that occurs because AWS credentials can't be found.
type CredentialsError struct {
	UnderlyingErr error
}

func (err CredentialsError) Error() string {
	return fmt.Sprintf("Error finding AWS credentials. Did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables or configure an AWS profile? Underlying error: %v", err.UnderlyingErr)
}

// ECRManifestFetchError is an error that occurs when retrieving information about a given tag in an ECR repository.
type ECRManifestFetchError struct {
	manifestURL string
	statusCode  int
	body        string
}

func (err ECRManifestFetchError) Error() string {
	return fmt.Sprintf("Error querying ECR repo URL %s (status code %d) (response body %s)", err.manifestURL, err.statusCode, err.body)
}
