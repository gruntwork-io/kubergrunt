package eksawshelper

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/kubergrunt/commonerrors"
	"github.com/hashicorp/go-cleanhttp"
)

// TagExistsInRepo queries the ECR repository docker API to see if the given tag exists for the given ECR repository.
func TagExistsInRepo(token, repoDomain, repoPath, tag string) (bool, error) {
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", repoDomain, repoPath, tag)
	req, err := http.NewRequest("GET", manifestURL, nil)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}
	req.Header.Set("Authorization", "Basic "+token)

	httpClient := cleanhttp.DefaultClient()
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	switch resp.StatusCode {
	case 200:
		return true, nil
	case 404:
		return false, nil
	}

	// All other status codes should be consider API errors.
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}
	return false, errors.WithStackTrace(ECRManifestFetchError{
		manifestURL: manifestURL,
		statusCode:  resp.StatusCode,
		body:        string(body),
	})
}

// GetDockerLoginToken retrieves an authorization token that can be used to access ECR via the docker APIs. The
// return token can directly be used as a HTTP authorization header for basic auth.
func GetDockerLoginToken(region string) (string, error) {
	client, err := NewECRClient(region)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	resp, err := client.GetAuthorizationToken(&ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if len(resp.AuthorizationData) != 1 {
		// AWS docs mention that there is always one token returned on a successful response.
		return "", errors.WithStackTrace(commonerrors.ImpossibleErr("AWS_DID_NOT_RETURN_DOCKER_TOKEN"))
	}
	return aws.StringValue(resp.AuthorizationData[0].AuthorizationToken), nil
}

// NewECRClient creates an AWS SDK client to access ECR API.
func NewECRClient(region string) (*ecr.ECR, error) {
	sess, err := NewAuthenticatedSession(region)
	if err != nil {
		return nil, err
	}
	return ecr.New(sess), nil
}
