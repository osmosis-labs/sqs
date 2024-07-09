package mocks

// MockTokenLoader is a mock implementation of TokenLoader.
type MockTokenLoader struct {
	callCount int
	Err       error
}

// FetchAndUpdateTokens implements the TokenLoader interface.
func (m *MockTokenLoader) FetchAndUpdateTokens() error {
	m.callCount++
	return m.Err
}

// CallCount returns the number of times FetchAndUpdateTokens was called.
func (m *MockTokenLoader) CallCount() int {
	return m.callCount
}
