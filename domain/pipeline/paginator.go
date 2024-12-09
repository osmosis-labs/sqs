package pipeline

import v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"

// NewPaginator initializes a Paginator with an Iterator
func NewPaginator[K, V any](iterator Iterator[K, V], p *v1beta1.PaginationRequest) *Paginator[K, V] {
	if p == nil {
		p = &v1beta1.PaginationRequest{}
	}

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
	if p.pagination.Strategy == v1beta1.PaginationStrategy_PAGE {
		return p.FetchPageByPageNumber()
	}
	return p.FetchPageByCursor()
}

// FetchPageByPageNumber retrieves elements for the current page based on page-based pagination strategy.
func (p *Paginator[K, V]) FetchPageByPageNumber() []V {
	// Ensure we're starting fresh
	p.iterator.Reset()

	// Set the offset based on the page number to avoid fetching data from the beginning
	p.iterator.SetOffset(int(p.pagination.Page * p.pagination.Limit))

	items := make([]V, 0, p.pagination.Limit)
	for i := uint64(0); i < p.pagination.Limit && p.iterator.HasNext(); i++ {
		elem, err := p.iterator.Next()
		if err == nil {
			items = append(items, elem)
		}
	}

	return items
}

// FetchPageByCursor retrieves elements for the current page based on cursor-based pagination strategy.
func (p *Paginator[K, V]) FetchPageByCursor() []V {
	// Ensure we're starting fresh
	p.iterator.Reset()

	// Set the offset based on the page number to avoid fetching data from the beginning
	p.iterator.SetOffset(int(p.pagination.Cursor))

	items := make([]V, 0, p.pagination.Limit)
	for i := uint64(0); i < p.pagination.Limit && p.iterator.HasNext(); i++ {
		elem, err := p.iterator.Next()
		if err == nil {
			items = append(items, elem)
		}
	}

	return items
}
