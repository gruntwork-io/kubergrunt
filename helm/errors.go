package helm

import (
	"fmt"
)

// MultiHelmError is returned when there are multiple errors in a helm action.
type MultiHelmError struct {
	Action string
	Errors []error
}

func (err MultiHelmError) Error() string {
	base := fmt.Sprintf("Multiple errors caught while performing helm action %s:\n", err.Action)
	for _, subErr := range err.Errors {
		subErrMessage := fmt.Sprintf("%s", subErr)
		base = base + subErrMessage + "\n"
	}
	return base
}

func (err MultiHelmError) AddError(newErr error) {
	err.Errors = append(err.Errors, newErr)
}

func (err MultiHelmError) IsEmpty() bool {
	return len(err.Errors) == 0
}
