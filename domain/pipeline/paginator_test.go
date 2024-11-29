package pipeline

import (
	"testing"

	v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"

	"github.com/stretchr/testify/require"
)

func TestGetPage(t *testing.T) {
	tests := []struct {
		name       string
		items      []int
		pagination *v1beta1.PaginationRequest
		want       []int
	}{
		{
			name:  "Page-based strategy with odd number of items",
			items: []int{1, 2, 3, 4, 5, 6, 7},
			pagination: &v1beta1.PaginationRequest{
				Strategy: v1beta1.PaginationStrategy_PAGE,
				Page:     1,
				Limit:    3,
			},
			want: []int{4, 5, 6},
		},
		{
			name:  "Cursor-based strategy with even number of items",
			items: []int{10, 20, 30, 40, 50, 60},
			pagination: &v1beta1.PaginationRequest{
				Strategy: v1beta1.PaginationStrategy_CURSOR,
				Cursor:   2,
				Limit:    3,
			},
			want: []int{30, 40, 50},
		},
		{
			name:  "Page-based strategy with limit exceeding remaining items",
			items: []int{1, 2, 3, 4, 5},
			pagination: &v1beta1.PaginationRequest{
				Strategy: v1beta1.PaginationStrategy_PAGE,
				Page:     1,
				Limit:    10,
			},
			want: []int{},
		},
		{
			name:  "Cursor-based strategy with cursor at last item",
			items: []int{100, 200, 300, 400},
			pagination: &v1beta1.PaginationRequest{
				Strategy: v1beta1.PaginationStrategy_CURSOR,
				Cursor:   3,
				Limit:    2,
			},
			want: []int{400},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iterator := &MockIterator{items: tt.items}
			paginator := NewPaginator[int, int](iterator, tt.pagination)
			got := paginator.GetPage()
			require.Equal(t, tt.want, got, "GetPage() returned unexpected result")
		})
	}
}

func TestFetchPageByPageNumber(t *testing.T) {
	tests := []struct {
		name       string
		items      []int
		pagination *v1beta1.PaginationRequest
		want       []int
	}{
		{
			name:  "First page of 3 items",
			items: []int{1, 2, 3, 4, 5},
			pagination: &v1beta1.PaginationRequest{
				Page:  0,
				Limit: 3,
			},
			want: []int{1, 2, 3},
		},
		{
			name:  "Second page of 2 items",
			items: []int{1, 2, 3, 4, 5},
			pagination: &v1beta1.PaginationRequest{
				Page:  1,
				Limit: 2,
			},
			want: []int{3, 4},
		},
		{
			name:  "Last page with fewer items than limit",
			items: []int{1, 2, 3, 4, 5},
			pagination: &v1beta1.PaginationRequest{
				Page:  1,
				Limit: 3,
			},
			want: []int{4, 5},
		},
		{
			name:  "Empty result for page beyond available items",
			items: []int{1, 2, 3},
			pagination: &v1beta1.PaginationRequest{
				Page:  2,
				Limit: 2,
			},
			want: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iterator := &MockIterator{items: tt.items}
			paginator := NewPaginator[int, int](iterator, tt.pagination)
			got := paginator.FetchPageByPageNumber()
			require.Equal(t, tt.want, got, "FetchPageByPageNumber() returned unexpected result")
		})
	}
}

func TestFetchPageByCursor(t *testing.T) {
	tests := []struct {
		name       string
		data       []int
		pagination *v1beta1.PaginationRequest
		expected   []int
	}{
		{
			name: "Fetch first page",
			data: []int{1, 2, 3, 4, 5},
			pagination: &v1beta1.PaginationRequest{
				Cursor: 0,
				Limit:  3,
			},
			expected: []int{1, 2, 3},
		},
		{
			name: "Fetch second page",
			data: []int{1, 2, 3, 4, 5},
			pagination: &v1beta1.PaginationRequest{
				Cursor: 3,
				Limit:  2,
			},
			expected: []int{4, 5},
		},
		{
			name: "Fetch beyond available data",
			data: []int{1, 2, 3, 4, 5},
			pagination: &v1beta1.PaginationRequest{
				Cursor: 5,
				Limit:  2,
			},
			expected: []int{},
		},
		{
			name: "Fetch with limit greater than remaining items",
			data: []int{1, 2, 3, 4, 5},
			pagination: &v1beta1.PaginationRequest{
				Cursor: 3,
				Limit:  5,
			},
			expected: []int{4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockIterator := &MockIterator{items: tt.data}
			paginator := NewPaginator[int, int](mockIterator, tt.pagination)
			got := paginator.FetchPageByCursor()
			require.Equal(t, tt.expected, got, "FetchPageByCursor() returned unexpected result")
		})
	}
}
