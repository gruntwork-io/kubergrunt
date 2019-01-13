package eks

import (
	"fmt"
	"testing"
	"time"

	awsgo "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/stretchr/testify/require"
)

func TestGetAsgByNameReturnsCorrectAsg(t *testing.T) {
	t.Parallel()

	uniqueID := random.UniqueId()
	name := fmt.Sprintf("%s-%s", t.Name(), uniqueID)
	otherUniqueID := random.UniqueId()
	otherName := fmt.Sprintf("%s-%s", t.Name(), otherUniqueID)

	region := getRandomRegion(t)
	asgSvc := aws.NewAsgClient(t, region)

	defer terminateEc2InstancesByName(t, region, []string{name, otherName})
	defer deleteAutoScalingGroup(t, name, region)
	defer deleteAutoScalingGroup(t, otherName, region)
	createTestAutoScalingGroup(t, name, region, 1)
	createTestAutoScalingGroup(t, otherName, region, 1)

	asg, err := GetAsgByName(asgSvc, name)
	require.NoError(t, err)
	require.Equal(t, *asg.AutoScalingGroupName, name)
}

func TestSetAsgCapacityDeploysNewInstances(t *testing.T) {
	t.Parallel()

	uniqueID := random.UniqueId()
	name := fmt.Sprintf("%s-%s", t.Name(), uniqueID)

	region := getRandomRegion(t)
	asgSvc := aws.NewAsgClient(t, region)

	defer terminateEc2InstancesByName(t, region, []string{name})
	defer deleteAutoScalingGroup(t, name, region)
	createTestAutoScalingGroup(t, name, region, 1)

	asg, err := GetAsgByName(asgSvc, name)
	require.NoError(t, err)
	existingInstances := asg.Instances

	require.NoError(t, setAsgCapacity(asgSvc, name, 2))
	require.NoError(t, waitForCapacity(asgSvc, name, 40, 15*time.Second))

	asg, err = GetAsgByName(asgSvc, name)
	allInstances := asg.Instances
	require.Equal(t, len(allInstances), len(existingInstances)+1)

	existingInstanceIds := idsFromAsgInstances(existingInstances)
	newInstanceIds, err := getLaunchedInstanceIds(asgSvc, name, existingInstanceIds)
	require.NoError(t, err)
	require.Equal(t, len(existingInstanceIds), 1)
	require.Equal(t, len(newInstanceIds), 1)
	require.NotEqual(t, existingInstanceIds[0], newInstanceIds[0])
}

func TestSetAsgCapacityRemovesInstances(t *testing.T) {
	t.Parallel()

	uniqueID := random.UniqueId()
	name := fmt.Sprintf("%s-%s", t.Name(), uniqueID)

	region := getRandomRegion(t)
	asgSvc := aws.NewAsgClient(t, region)

	defer terminateEc2InstancesByName(t, region, []string{name})
	defer deleteAutoScalingGroup(t, name, region)
	createTestAutoScalingGroup(t, name, region, 2)

	asg, err := GetAsgByName(asgSvc, name)
	require.NoError(t, err)
	existingInstances := asg.Instances

	require.NoError(t, setAsgCapacity(asgSvc, name, 1))
	require.NoError(t, waitForCapacity(asgSvc, name, 40, 15*time.Second))

	asg, err = GetAsgByName(asgSvc, name)
	allInstances := asg.Instances
	require.Equal(t, len(allInstances), len(existingInstances)-1)
}

// The following functions were adapted from the tests for cloud-nuke

func getRandomRegion(t *testing.T) string {
	// Use the same regions as those that EKS is available
	approvedRegions := []string{"us-west-2", "us-east-1", "us-east-2", "eu-west-1"}
	return aws.GetRandomRegion(t, approvedRegions, []string{})
}

func createTestAutoScalingGroup(t *testing.T, name string, region string, desiredCount int64) {
	instance := createTestEC2Instance(t, region, name)

	asgClient := aws.NewAsgClient(t, region)
	param := &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName: &name,
		InstanceId:           instance.InstanceId,
		DesiredCapacity:      awsgo.Int64(desiredCount),
		MinSize:              awsgo.Int64(1),
		MaxSize:              awsgo.Int64(3),
	}
	_, err := asgClient.CreateAutoScalingGroup(param)
	require.NoError(t, err)

	err = asgClient.WaitUntilGroupExists(&autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{&name},
	})
	require.NoError(t, err)

	aws.WaitForCapacity(t, name, region, 40, 15*time.Second)
}

func createTestEC2Instance(t *testing.T, region string, name string) ec2.Instance {
	ec2Client := aws.NewEc2Client(t, region)
	imageID := aws.GetAmazonLinuxAmi(t, region)
	params := &ec2.RunInstancesInput{
		ImageId:      awsgo.String(imageID),
		InstanceType: awsgo.String("t2.micro"),
		MinCount:     awsgo.Int64(1),
		MaxCount:     awsgo.Int64(1),
	}
	runResult, err := ec2Client.RunInstances(params)
	require.NoError(t, err)

	require.NotEqual(t, len(runResult.Instances), 0)

	err = ec2Client.WaitUntilInstanceExists(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   awsgo.String("instance-id"),
				Values: []*string{runResult.Instances[0].InstanceId},
			},
		},
	})
	require.NoError(t, err)

	// Add test tag to the created instance
	_, err = ec2Client.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{runResult.Instances[0].InstanceId},
		Tags: []*ec2.Tag{
			{
				Key:   awsgo.String("Name"),
				Value: awsgo.String(name),
			},
		},
	})
	require.NoError(t, err)

	// EC2 Instance must be in a running before this function returns
	err = ec2Client.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   awsgo.String("instance-id"),
				Values: []*string{runResult.Instances[0].InstanceId},
			},
		},
	})
	require.NoError(t, err)

	return *runResult.Instances[0]
}

func terminateEc2InstancesByName(t *testing.T, region string, names []string) {
	for _, name := range names {
		instanceIds := aws.GetEc2InstanceIdsByTag(t, region, "Name", name)
		for _, instanceID := range instanceIds {
			aws.TerminateInstance(t, region, instanceID)
		}
	}
}

func deleteAutoScalingGroup(t *testing.T, name string, region string) {
	// We have to scale ASG down to 0 before we can delete it
	scaleAsgToZero(t, name, region)

	asgClient := aws.NewAsgClient(t, region)
	input := &autoscaling.DeleteAutoScalingGroupInput{AutoScalingGroupName: awsgo.String(name)}
	_, err := asgClient.DeleteAutoScalingGroup(input)
	require.NoError(t, err)
	err = asgClient.WaitUntilGroupNotExists(&autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{awsgo.String(name)},
	})
	require.NoError(t, err)
}

func scaleAsgToZero(t *testing.T, name string, region string) {
	asgClient := aws.NewAsgClient(t, region)
	input := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: awsgo.String(name),
		DesiredCapacity:      awsgo.Int64(0),
		MinSize:              awsgo.Int64(0),
		MaxSize:              awsgo.Int64(0),
	}
	_, err := asgClient.UpdateAutoScalingGroup(input)
	require.NoError(t, err)
	aws.WaitForCapacity(t, name, region, 40, 15*time.Second)

	// There is an eventual consistency bug where even though the ASG is scaled down, AWS sometimes still views a
	// scaling activity so we add a 5 second pause here to work around it.
	time.Sleep(5 * time.Second)
}
