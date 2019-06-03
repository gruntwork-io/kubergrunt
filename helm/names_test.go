package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeLabelValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"ReplaceUnsupportedChar",
			"foo@bar",
			"foo-bar",
		},
		{
			"PreserveSupportedChars",
			"foo-bar",
			"foo-bar",
		},
		{
			"PreserveCaps",
			"FOO@BAR",
			"FOO-BAR",
		},
		{
			"EmptyString",
			"",
			"",
		},
		{
			"MultipleUnsupportedChars",
			"team@gruntwork.io$foobar",
			"team-gruntwork.io-foobar",
		},
		{
			"Numbers",
			"1337",
			"1337",
		},
	}

	for _, testCase := range testCases {
		// Capture range variable to bring it in scope of the for loop
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, sanitizeLabelValues(testCase.input), testCase.expected)
		})
	}
}

func TestGetTillerRoleLabelsSanitizesValues(t *testing.T) {
	t.Parallel()

	labels := getTillerRoleLabels("foo@bar", "default")
	assert.Equal(t, labels[EntityIDLabel], "foo-bar")
}

func TestGetTillerRoleBindingLabelsSanitizesValues(t *testing.T) {
	t.Parallel()

	labels := getTillerRoleBindingLabels("foo@bar", "default")
	assert.Equal(t, labels[EntityIDLabel], "foo-bar")
}
