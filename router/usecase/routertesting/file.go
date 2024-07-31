package routertesting

import "os"

// MustReadFile reads a file and panics if there is an error.
func (s *RouterTestHelper) MustReadFile(path string) string {
	b, err := os.ReadFile(path)
	s.Require().NoError(err)
	return string(b)
}
