package kubectl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAWSLoadBalancerNameFromHostname(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		hostname       string
		expectedLBName string
		expectErr      bool
	}{
		{"foo.bar.com", "", true},
		{
			"k8s-kubesyst-agtvv3pn-73e284744f-1915683655.ap-northeast-1.elb.amazonaws.com",
			"k8s-kubesyst-agtvv3pn-73e284744f",
			false,
		},
		{
			"73e284744f-1915683655.ap-northeast-1.elb.amazonaws.com",
			"73e284744f",
			false,
		},
		{
			"internal-73e284744f-1915683655.ap-northeast-1.elb.amazonaws.com",
			"73e284744f",
			false,
		},
		{
			"internal-k8s-kubesyst-agtvv3pn-73e284744f-1915683655.ap-northeast-1.elb.amazonaws.com",
			"k8s-kubesyst-agtvv3pn-73e284744f",
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.hostname, func(t *testing.T) {
			lbName, err := getAWSLoadBalancerNameFromHostname(tc.hostname)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedLBName, lbName)
			}
		})
	}
}
