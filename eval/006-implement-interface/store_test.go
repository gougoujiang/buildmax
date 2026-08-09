package store

import "testing"

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	return &FileStore{Dir: t.TempDir()}
}

func TestSetGet(t *testing.T) {
	s := newTestStore(t)
	if err := s.Set("greeting", "hello"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, err := s.Get("greeting")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "hello" {
		t.Errorf("Get = %q, want %q", val, "hello")
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("missing")
	if err != ErrNotFound {
		t.Errorf("Get missing key: err = %v, want ErrNotFound", err)
	}
}

func TestSetOverwrite(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("k", "first")
	_ = s.Set("k", "second")
	val, err := s.Get("k")
	if err != nil || val != "second" {
		t.Errorf("after overwrite Get = %q, %v; want second, nil", val, err)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("k", "v")
	if err := s.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Exists("k") {
		t.Error("key should not exist after Delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete("missing"); err != ErrNotFound {
		t.Errorf("Delete missing: err = %v, want ErrNotFound", err)
	}
}

func TestExists(t *testing.T) {
	s := newTestStore(t)
	if s.Exists("k") {
		t.Error("Exists before Set should be false")
	}
	_ = s.Set("k", "v")
	if !s.Exists("k") {
		t.Error("Exists after Set should be true")
	}
	_ = s.Delete("k")
	if s.Exists("k") {
		t.Error("Exists after Delete should be false")
	}
}
