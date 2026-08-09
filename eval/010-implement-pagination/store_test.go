package items

import "testing"

func TestPage_FirstPage(t *testing.T) {
	s := NewStore(10)
	page := s.Page(0, 3)
	if len(page) != 3 {
		t.Fatalf("Page(0,3) len = %d, want 3", len(page))
	}
	if page[0].ID != 1 || page[2].ID != 3 {
		t.Errorf("Page(0,3) IDs = %d..%d, want 1..3", page[0].ID, page[2].ID)
	}
}

func TestPage_MiddlePage(t *testing.T) {
	s := NewStore(10)
	page := s.Page(4, 3)
	if len(page) != 3 {
		t.Fatalf("Page(4,3) len = %d, want 3", len(page))
	}
	if page[0].ID != 5 {
		t.Errorf("Page(4,3)[0].ID = %d, want 5", page[0].ID)
	}
}

func TestPage_PartialLastPage(t *testing.T) {
	s := NewStore(10)
	page := s.Page(8, 5) // only 2 items remain
	if len(page) != 2 {
		t.Fatalf("Page(8,5) len = %d, want 2", len(page))
	}
	if page[0].ID != 9 || page[1].ID != 10 {
		t.Errorf("Page(8,5) IDs = %d,%d, want 9,10", page[0].ID, page[1].ID)
	}
}

func TestPage_OffsetBeyondEnd(t *testing.T) {
	s := NewStore(5)
	page := s.Page(10, 3)
	if len(page) != 0 {
		t.Fatalf("Page(10,3) len = %d, want 0", len(page))
	}
}

func TestPage_ExactLastPage(t *testing.T) {
	s := NewStore(6)
	page := s.Page(3, 3)
	if len(page) != 3 || page[0].ID != 4 || page[2].ID != 6 {
		t.Errorf("Page(3,3) = %v", page)
	}
}

func TestPage_ZeroLimit(t *testing.T) {
	s := NewStore(5)
	page := s.Page(0, 0)
	if len(page) != 0 {
		t.Fatalf("Page(0,0) len = %d, want 0", len(page))
	}
}
