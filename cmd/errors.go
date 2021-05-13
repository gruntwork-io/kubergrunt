package main

import "fmt"

// MutualExclusiveFlagError is returned when there is a violation of a mutually exclusive flag set.
type MutuallyExclusiveFlagError struct {
	Message string
}

func (err MutuallyExclusiveFlagError) Error() string {
	return err.Message
}

// ExactlyOneASGErr is returned if a user does not provide exactly one ASG.
type ExactlyOneASGErr struct {
	flagName string
}

func (err ExactlyOneASGErr) Error() string {
	return fmt.Sprintf("You must provide exactly one ASG using %s to this command.", err.flagName)
}
