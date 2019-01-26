package main

// MutualExclusiveFlagError is returned when there is a violation of a mutually exclusive flag set.
type MutuallyExclusiveFlagError struct {
	Message string
}

func (err MutuallyExclusiveFlagError) Error() string {
	return err.Message
}
