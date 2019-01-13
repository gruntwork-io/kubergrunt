package eks

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/require"
)

func TestTerminateInstances(t *testing.T) {
	t.Parallel()

	uniqueID := random.UniqueId()
	name := fmt.Sprintf("%s-%s", t.Name(), uniqueID)
	region := getRandomRegion(t)
	ec2Svc := aws.NewEc2Client(t, region)
	instance := createTestEC2Instance(t, region, name)
	terminateInstances(ec2Svc, []string{*instance.InstanceId})
	instanceIds := aws.GetEc2InstanceIdsByTag(t, region, "Name", name)
	instances, err := instanceDetailsFromIds(ec2Svc, instanceIds)
	require.NoError(t, err)

	// We want either no instances, or the instance is in terminated state
	if len(instances) > 0 {
		instance := instances[0]
		require.Equal(t, *instance.State.Name, "terminated")
	}
}

func TestInstanceDetailsFromIds(t *testing.T) {
	t.Parallel()

	uniqueID := random.UniqueId()
	name := fmt.Sprintf("%s-%s", t.Name(), uniqueID)
	region := getRandomRegion(t)
	ec2Svc := aws.NewEc2Client(t, region)
	instance := createTestEC2Instance(t, region, name)
	defer aws.TerminateInstance(t, region, *instance.InstanceId)

	instances, err := instanceDetailsFromIds(ec2Svc, []string{*instance.InstanceId})
	require.NoError(t, err)
	require.Equal(t, len(instances), 1)
	require.Equal(t, *instances[0].Tags[0].Value, name)
}
