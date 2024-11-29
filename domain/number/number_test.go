package number

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNumbers(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []uint64
		wantErr bool
	}{
		{
			name:  "valid numbers",
			input: "1, 2, 3, 4, 5",
			want:  []uint64{1, 2, 3, 4, 5},
		},
		{
			name:  "single number",
			input: "42",
			want:  []uint64{42},
		},
		{
			name:  "large numbers",
			input: "1000000, 2000000, 3000000",
			want:  []uint64{1000000, 2000000, 3000000},
		},
		{
			name:    "invalid number",
			input:   "1, 2, 3, abc, 5",
			wantErr: true,
		},
		{
			name:  "numbers with extra spaces",
			input: "  1  ,  2  ,  3  ",
			want:  []uint64{1, 2, 3},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "only commas",
			input: ",,,",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNumbers(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseNumberType(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		parseFunc   func(string) (any, error)
		expected    any
		expectError bool
	}{
		{
			name:  "Parse uint64 numbers",
			input: "1, 2, 3",
			parseFunc: func(s string) (any, error) {
				return strconv.ParseUint(s, 10, 64)
			},
			expected:    []uint64{1, 2, 3},
			expectError: false,
		},
		{
			name:  "Parse int numbers",
			input: "-1, 0, 1",
			parseFunc: func(s string) (any, error) {
				return strconv.Atoi(s)
			},
			expected:    []int{-1, 0, 1},
			expectError: false,
		},
		{
			name:  "Parse float64 numbers",
			input: "1.1, 2.2, 3.3",
			parseFunc: func(s string) (any, error) {
				return strconv.ParseFloat(s, 64)
			},
			expected:    []float64{1.1, 2.2, 3.3},
			expectError: false,
		},
		{
			name:  "Invalid input for uint64",
			input: "1, 2, -3",
			parseFunc: func(s string) (any, error) {
				return strconv.ParseUint(s, 10, 64)
			},
			expected:    []uint64{},
			expectError: true,
		},
		{
			name:  "Empty input",
			input: "",
			parseFunc: func(s string) (any, error) {
				return strconv.Atoi(s)
			},
			expected:    nil,
			expectError: false,
		},
		{
			name:  "Input with spaces",
			input: "  1  ,  2  ,  3  ",
			parseFunc: func(s string) (any, error) {
				return strconv.Atoi(s)
			},
			expected:    []int{1, 2, 3},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result any
			var err error

			switch tt.expected.(type) {
			case []uint64:
				result, err = ParseNumberType(tt.input, func(s string) (uint64, error) {
					return strconv.ParseUint(s, 10, 64)
				})
			case []int:
				result, err = ParseNumberType(tt.input, func(s string) (int, error) {
					return strconv.Atoi(s)
				})
			case []float64:
				result, err = ParseNumberType(tt.input, func(s string) (float64, error) {
					return strconv.ParseFloat(s, 64)
				})
			}

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}
