module github.com/gruntwork-io/kubergrunt

go 1.14

require (
	github.com/aws/aws-sdk-go v1.38.14
	github.com/blang/semver/v4 v4.0.0
	github.com/gruntwork-io/go-commons v0.8.2
	github.com/gruntwork-io/terratest v0.32.9
	github.com/mitchellh/go-homedir v1.1.0
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/urfave/cli v1.22.4

	// EKS is still on k8s v1.17
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.19.3
	k8s.io/client-go v0.19.3
	sigs.k8s.io/aws-iam-authenticator v0.5.1
)
