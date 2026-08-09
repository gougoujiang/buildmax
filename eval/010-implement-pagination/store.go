package items

import "fmt"

// Item is a single record in the store.
type Item struct {
	ID   int
	Name string
}

// Store holds items in memory.
type Store struct {
	data []Item
}

// NewStore creates a Store pre-populated with n items (IDs 1..n).
func NewStore(n int) *Store {
	s := &Store{}
	for i := 1; i <= n; i++ {
		s.data = append(s.data, Item{ID: i, Name: fmt.Sprintf("item-%d", i)})
	}
	return s
}

// Add appends an item to the store.
func (s *Store) Add(item Item) {
	s.data = append(s.data, item)
}

// List returns all items.
func (s *Store) List() []Item {
	return s.data
}

// Count returns the total number of items.
func (s *Store) Count() int {
	return len(s.data)
}

// Page returns at most limit items starting at zero-based offset.
// Returns an empty (non-nil) slice when offset >= Count().
// If offset+limit exceeds Count(), only the remaining items are returned.
func (s *Store) Page(offset, limit int) []Item {
	// TODO: implement
	return []Item{}
}
