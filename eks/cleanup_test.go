package eks

import (
	"io/ioutil"
	"testing"

	awsgo "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gruntwork-io/kubergrunt/eksawshelper"
	"github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/terraform"
	test_structure "github.com/gruntwork-io/terratest/modules/test-structure"
	"github.com/stretchr/testify/require"
)

const cleanupTestCasesFolder = "./fixture/cleanup-test"

func TestDeleteSecurityGroupDependencies(t *testing.T) {
	t.Parallel()

	dirs, err := ioutil.ReadDir(cleanupTestCasesFolder)
	require.NoError(t, err)

	for _, dir := range dirs {
		dirName := dir.Name()
		t.Run(dirName, func(t *testing.T) {
			t.Parallel()
			testSGDependencyCleanup(t, dirName)
		})
	}

}

func testSGDependencyCleanup(t *testing.T, exampleName string) {
	exampleFolder := test_structure.CopyTerraformFolderToTemp(t, cleanupTestCasesFolder, exampleName)
	awsRegion := aws.GetRandomStableRegion(t, nil, nil)
	opts := &terraform.Options{
		TerraformDir: exampleFolder,
		Vars: map[string]interface{}{
			"prefix": random.UniqueId(),
		},
		EnvVars: map[string]string{
			"AWS_DEFAULT_REGION": awsRegion,
		},
	}

	// We use the E flavor to ignore errors, since the resources will already be destroyed in the successful case, which
	// may interfere with the destroy call. Note that we still want to call destroy in case the cleanup routine is not
	// functioning.
	defer terraform.DestroyE(t, opts)
	terraform.InitAndApply(t, opts)

	securityGroupId := terraform.OutputRequired(t, opts, "security_group_id")
	sess, err := eksawshelper.NewAuthenticatedSession(awsRegion)
	require.NoError(t, err)
	ec2Svc := ec2.New(sess)
	require.NoError(t, deleteDependencies(ec2Svc, securityGroupId))

	networkInterfaceId := terraform.OutputRequired(t, opts, "eni_id")
	describeNetworkInterfacesInput := &ec2.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: awsgo.StringSlice([]string{networkInterfaceId}),
	}
	_, err = ec2Svc.DescribeNetworkInterfaces(describeNetworkInterfacesInput)
	require.Error(t, err)

	// Make sure it is the not found error
	awsErr, isAwsErr := err.(awserr.Error)
	require.True(t, isAwsErr)
	require.Equal(t, awsErr.Code(), "InvalidNetworkInterfaceID.NotFound")

}
