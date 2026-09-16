package service

import "fmt"

// ErrInvalidInput is returned by service methods for request-shape /
// business-rule violations, as distinct from repository.ErrNotFound or
// unexpected infrastructure errors. Handlers type-switch on this to
// decide between a 400 and a 500.
type ErrInvalidInput struct {
	Field  string
	Reason string
}

func (e ErrInvalidInput) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Reason)
}
