// Package commonerrors contains error types that are common across the project.
package commonerrors

import "fmt"

// ImpossibleErr is returned for impossible conditions that should never happen in the code. This error should only be
// returned if there is no user remedy and represents a bug in the code.
type ImpossibleErr string

func (err ImpossibleErr) Error() string {
	return fmt.Sprintf(
		"You reached a point in kubergrunt that should not happen and is almost certainly a bug. Please open a GitHub issue on https://github.com/gruntwork-io/kubergrunt/issues with the contents of this error message. Code: %s",
		string(err),
	)
}
