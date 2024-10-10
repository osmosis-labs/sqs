// Package slices provides utility functions for working with slices.
package slices

// Split splits given slice into chunks of specified size.
// Returns a slice of slices, where each inner slice is a chunk of the original slice of given size.
// The last chunk may be smaller than the specified size if the original slice length is not evenly divisible by the chunk size.
func Split[T any](s []T, size int) [][]T {
	var result [][]T

	for l := 0; l < len(s); l += size {
		h := l + size
		if h > len(s) {
			h = len(s)
		}
		result = append(result, s[l:h])
	}

	return result
}
