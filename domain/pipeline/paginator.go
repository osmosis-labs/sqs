package pipeline

import v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"

// NewPaginator initializes a Paginator with an Iterator
func NewPaginator[K, V any](iterator Iterator[K, V], p *v1beta1.PaginationRequest) *Paginator[K, V] {
	return &Paginator[K, V]{
		iterator:   iterator,
		pagination: p,
	}
}

// Paginator relies on Iterator to fetch paginated data without knowing the data type
type Paginator[K, V any] struct {
	iterator   Iterator[K, V]
	pagination *v1beta1.PaginationRequest
}

// GetPage retrieves elements for the current page based on pagination strategy.
// Under the hood it calls either GetPageBasedPage or GetCursorBasedPage.
func (p *Paginator[K, V]) GetPage() []V {
	return p.FetchPageByPageNumber()
}

// FetchPageByPageNumber retrieves elements for the current page based on page-based pagination strategy.
func (p *Paginator[K, V]) FetchPageByPageNumber() []V {
	p.iterator.Reset() // Ensure we're starting fresh
	start := p.pagination.Page * p.pagination.Limit
	items := make([]V, 0, p.pagination.Limit)

	for i := uint64(0); i < start+p.pagination.Limit && p.iterator.HasNext(); i++ { // this is quite inefficient, we should be able to set the start index
		elem, valid := p.iterator.Next()
		if valid && i >= start {
			items = append(items, elem)
		}
	}

	return items
}
