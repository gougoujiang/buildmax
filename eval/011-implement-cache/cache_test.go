package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSetGet(t *testing.T) {
	c := New(0)
	defer c.Close()
	c.Set("k", "v", 0)
	val, ok := c.Get("k")
	if !ok || val != "v" {
		t.Errorf("Get = %q, %v; want v, true", val, ok)
	}
}

func TestGetMissing(t *testing.T) {
	c := New(0)
	defer c.Close()
	_, ok := c.Get("missing")
	if ok {
		t.Error("Get missing key should return false")
	}
}

func TestDelete(t *testing.T) {
	c := New(0)
	defer c.Close()
	c.Set("k", "v", 0)
	c.Delete("k")
	_, ok := c.Get("k")
	if ok {
		t.Error("Get after Delete should return false")
	}
}

func TestTTL_Expired(t *testing.T) {
	c := New(0)
	defer c.Close()
	c.Set("k", "v", 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	_, ok := c.Get("k")
	if ok {
		t.Error("expired key should not be returned by Get")
	}
}

func TestTTL_NotYetExpired(t *testing.T) {
	c := New(0)
	defer c.Close()
	c.Set("k", "v", 500*time.Millisecond)
	val, ok := c.Get("k")
	if !ok || val != "v" {
		t.Errorf("unexpired key: Get = %q, %v; want v, true", val, ok)
	}
}

func TestLen(t *testing.T) {
	c := New(0)
	defer c.Close()
	if c.Len() != 0 {
		t.Errorf("Len() = %d; want 0", c.Len())
	}
	c.Set("a", "1", 0)
	c.Set("b", "2", 0)
	if c.Len() != 2 {
		t.Errorf("Len() = %d; want 2", c.Len())
	}
	c.Delete("a")
	if c.Len() != 1 {
		t.Errorf("Len() after Delete = %d; want 1", c.Len())
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New(10 * time.Millisecond)
	defer c.Close()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i)
			c.Set(key, fmt.Sprintf("v%d", i), 0)
			c.Get(key)
			c.Delete(key)
		}(i)
	}
	wg.Wait()
}
