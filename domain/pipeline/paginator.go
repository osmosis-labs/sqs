package pipeline

// NewPaginator initializes a Paginator with an Iterator
func NewPaginator[K, V any](iterator Iterator[K, V], pageSize int) *Paginator[K, V] {
	return &Paginator[K, V]{
		iterator: iterator,
		pageSize: pageSize,
	}
}

// Paginator relies on Iterator to fetch paginated data without knowing the data type
type Paginator[K, V any] struct {
	iterator Iterator[K, V]
	pageSize int
}

// GetPage retrieves elements for the current page
func (p *Paginator[K, V]) GetPage(page int) []V {
	p.iterator.Reset() // Ensure we're starting fresh
	start := page * p.pageSize
	items := make([]V, 0, p.pageSize)

	for i := 0; i < start+p.pageSize && p.iterator.HasNext(); i++ { // this is quite inefficient, we should be able to set the start index
		elem, valid := p.iterator.Next()
		if valid && i >= start {
			items = append(items, elem)
		}
	}

	return items
}
