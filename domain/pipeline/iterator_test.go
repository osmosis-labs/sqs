package pipeline

// MockIterator is a simple implementation of Iterator for testing
type MockIterator struct {
	items []int
	index int
}

func (m *MockIterator) HasNext() bool {
	return m.index < len(m.items)
}

func (m *MockIterator) SetOffset(offset int) {
	m.index = offset
}

func (m *MockIterator) Next() (int, bool) {
	if m.HasNext() {
		item := m.items[m.index]
		m.index++
		return item, true
	}
	return 0, false
}

func (m *MockIterator) Reset() {
	m.index = 0
}
