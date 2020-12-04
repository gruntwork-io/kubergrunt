package retry

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDoWithRetry(t *testing.T) {
	t.Parallel()

	expectedOutput := "expected"
	expectedError := fmt.Errorf("expected error")

	actionAlwaysReturnsExpected := func() (interface{}, error) { return expectedOutput, nil }
	actionAlwaysReturnsError := func() (interface{}, error) { return expectedOutput, expectedError }

	createActionThatReturnsExpectedAfterFiveRetries := func() func() (interface{}, error) {
		count := 0
		return func() (interface{}, error) {
			count++
			if count > 5 {
				return expectedOutput, nil
			}
			return expectedOutput, expectedError
		}
	}

	testCases := []struct {
		description   string
		maxRetries    int
		expectedError error
		action        func() (interface{}, error)
	}{
		{"Return value on first try", 10, nil, actionAlwaysReturnsExpected},
		{"Return error on all retries", 10, MaxRetriesExceeded{Description: "Return error on all retries", MaxRetries: 10}, actionAlwaysReturnsError},
		{"Return value after 5 retries", 10, nil, createActionThatReturnsExpectedAfterFiveRetries()},
		{"Return value after 5 retries, but only do 4 retries", 4, MaxRetriesExceeded{Description: "Return value after 5 retries, but only do 4 retries", MaxRetries: 4}, createActionThatReturnsExpectedAfterFiveRetries()},
	}

	for _, testCase := range testCases {
		testCase := testCase // capture range variable for each test case

		t.Run(testCase.description, func(t *testing.T) {
			t.Parallel()

			actualOutput, err := DoWithRetryInterfaceE(testCase.description, testCase.maxRetries, 1*time.Millisecond, testCase.action)
			assert.Equal(t, expectedOutput, actualOutput)
			if testCase.expectedError != nil {
				assert.Equal(t, testCase.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, expectedOutput, actualOutput)
			}
		})
	}
}

type ErrorCounter int

func (count ErrorCounter) Error() string {
	return fmt.Sprintf("%d", int(count))
}
