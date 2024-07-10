package mocks

// MockError represents a type implementing the error interface for testing purposes.
type MockError struct {
	Err string
}

// Error returns the error message.
func (e MockError) Error() string {
	return e.Err
}
