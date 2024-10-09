package slices

// Split splits slice into chunks of specified size.
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
