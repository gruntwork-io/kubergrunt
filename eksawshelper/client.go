package eksawshelper

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

// NewAuthenticatedSession gets an AWS Session, checking that the user has credentials properly configured in their environment.
func NewAuthenticatedSession(region string) (*session.Session, error) {
	opts := session.Options{
		Config:            *(aws.NewConfig().WithRegion(region)),
		SharedConfigState: session.SharedConfigEnable,
	}
	sess, err := session.NewSessionWithOptions(opts)
	if err != nil {
		return nil, err
	}

	if _, err = sess.Config.Credentials.Get(); err != nil {
		return nil, CredentialsError{UnderlyingErr: err}
	}

	return sess, nil
}
