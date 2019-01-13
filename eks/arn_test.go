package eks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetClusterNameFromArn(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		in  string
		out string
	}{
		{"arn:aws:eks:us-east-2:111111111111:cluster/eks-cluster-srlBd2", "eks-cluster-srlBd2"},
		{"arn:aws:eks:us-east-2:111111111111:cluster/eks-cluster/srlBd2", "eks-cluster/srlBd2"},
	}
	for _, testcase := range testCases {
		t.Run(testcase.out, func(t *testing.T) {
			t.Parallel()

			name, err := GetClusterNameFromArn(testcase.in)
			assert.NoError(t, err)
			assert.Equal(t, name, testcase.out)
		})
	}
}

func TestGetClusterNameFromArnErrorCases(t *testing.T) {
	t.Parallel()

	var testCases = []string{
		"eks-cluster-srlBd2",
		"",
		"aws:eks:us-east-2:111111111111:cluster/eks-cluster/srlBd2",
	}
	for _, testcase := range testCases {
		t.Run(testcase, func(t *testing.T) {
			t.Parallel()

			name, err := GetClusterNameFromArn(testcase)
			assert.Error(t, err)
			assert.Equal(t, name, "")
		})
	}
}

func TestGetRegionFromArn(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		in  string
		out string
	}{
		{"arn:aws:eks:us-east-2:111111111111:cluster/eks-cluster-srlBd2", "us-east-2"},
		{"arn:aws:eks:eu-west-1:111111111111:cluster/eks-cluster/srlBd2", "eu-west-1"},
		{"arn:aws:eks::111111111111:cluster/eks-cluster/srlBd2", ""},
	}
	for _, testcase := range testCases {
		t.Run(testcase.out, func(t *testing.T) {
			t.Parallel()

			region, err := GetRegionFromArn(testcase.in)
			assert.NoError(t, err)
			assert.Equal(t, region, testcase.out)
		})
	}
}

func TestGetRegionFromArnErrorCases(t *testing.T) {
	t.Parallel()

	var testCases = []string{
		"eks-cluster-srlBd2",
		"",
		"aws:eks:us-east-2:111111111111:cluster/eks-cluster/srlBd2",
	}
	for _, testcase := range testCases {
		t.Run(testcase, func(t *testing.T) {
			t.Parallel()

			name, err := GetClusterNameFromArn(testcase)
			assert.Error(t, err)
			assert.Equal(t, name, "")
		})
	}
}
