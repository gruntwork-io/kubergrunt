package eks

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"

	"github.com/gruntwork-io/kubergrunt/logging"
)

// Given a list of instance IDs, fetch the instance details from AWS.
func instanceDetailsFromIds(svc *ec2.EC2, idList []string) ([]*ec2.Instance, error) {
	input := ec2.DescribeInstancesInput{InstanceIds: aws.StringSlice(idList)}
	instances := []*ec2.Instance{}
	// Handle pagination by repeatedly making the API call while there is a next token set.
	for {
		response, err := svc.DescribeInstances(&input)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		for _, reservation := range response.Reservations {
			instances = append(instances, reservation.Instances...)
		}
		if response.NextToken == nil {
			break
		}
		input.NextToken = response.NextToken
	}
	return instances, nil
}

// Currently EKS defaults to using the private DNS name for the node names.
// TODO: The property used should be configurable for deployments where the node names are custom.
func kubeNodeNamesFromInstances(instances []*ec2.Instance) []string {
	nodeNames := []string{}
	for _, inst := range instances {
		nodeNames = append(nodeNames, *inst.PrivateDnsName)
	}
	return nodeNames
}

// terminateInstances will make a call to EC2 API to terminate the instances provided in the list.
func terminateInstances(ec2Svc *ec2.EC2, idList []string) error {
	logger := logging.GetProjectLogger()
	logger.Infof("Terminating %d instances, in groups of up to 1000 instances", len(idList))

	// Batch the requests up to the limit of 1000 instances
	errList := NewMultipleTerminateInstanceErrors()
	for batchIdx, batchedInstanceIdList := range collections.BatchListIntoGroupsOf(idList, 1000) {
		instanceIds := aws.StringSlice(batchedInstanceIdList)
		input := &ec2.TerminateInstancesInput{
			InstanceIds: instanceIds,
		}
		_, err := ec2Svc.TerminateInstances(input)
		if err != nil {
			errList.AddError(err)
			logger.Errorf("Encountered error terminating instances in batch %d: %s", batchIdx, err)
			logger.Errorf("Instance ids: %s", strings.Join(batchedInstanceIdList, ","))
			continue
		}

		logger.Infof("Terminated %d instances from batch %d", len(batchedInstanceIdList), batchIdx)

		logger.Infof("Waiting for %d instances to shut down from batch %d", len(batchedInstanceIdList), batchIdx)
		err = ec2Svc.WaitUntilInstanceTerminated(&ec2.DescribeInstancesInput{InstanceIds: instanceIds})
		if err != nil {
			errList.AddError(err)
			logger.Errorf("Encountered error waiting for instances to shutdown in batch %d: %s", batchIdx, err)
			logger.Errorf("Instance ids: %s", strings.Join(batchedInstanceIdList, ","))
			continue
		}
		logger.Infof("Successfully shutdown %d instances from batch %d", len(batchedInstanceIdList), batchIdx)
	}
	if !errList.IsEmpty() {
		return errors.WithStackTrace(errList)
	}
	logger.Infof("Successfully shutdown all %d instances", len(idList))
	return nil
}
