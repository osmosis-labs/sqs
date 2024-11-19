package pipeline

import (
	"reflect"
	"testing"

	v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"
)

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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchPageByPageNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}
