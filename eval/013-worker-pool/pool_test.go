package workerpool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_AllJobsRun(t *testing.T) {
	p := New(4)
	var count int64
	for i := 0; i < 100; i++ {
		p.Submit(func() {
			atomic.AddInt64(&count, 1)
		})
	}
	p.Wait()
	if got := atomic.LoadInt64(&count); got != 100 {
		t.Errorf("ran %d jobs, want 100", got)
	}
	p.Close()
}

func TestPool_Wait_BeforeClose(t *testing.T) {
	p := New(2)
	var done int64
	for i := 0; i < 20; i++ {
		p.Submit(func() {
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&done, 1)
		})
	}
	p.Wait()
	if atomic.LoadInt64(&done) != 20 {
		t.Errorf("Wait returned before all jobs finished: done=%d", done)
	}
	p.Close()
}

func TestPool_ConcurrentSubmit(t *testing.T) {
	p := New(8)
	var wg sync.WaitGroup
	var count int64
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				p.Submit(func() {
					atomic.AddInt64(&count, 1)
				})
			}
		}()
	}
	wg.Wait()
	p.Wait()
	if atomic.LoadInt64(&count) != 100 {
		t.Errorf("count = %d, want 100", count)
	}
	p.Close()
}

func TestPool_CloseFlushesQueue(t *testing.T) {
	p := New(2)
	var count int64
	for i := 0; i < 50; i++ {
		p.Submit(func() {
			atomic.AddInt64(&count, 1)
		})
	}
	p.Close() // Close must wait for all pending jobs
	if atomic.LoadInt64(&count) != 50 {
		t.Errorf("after Close, count = %d, want 50", count)
	}
}

func TestPool_SingleWorker(t *testing.T) {
	p := New(1)
	results := make([]int, 0, 5)
	var mu sync.Mutex
	for i := 0; i < 5; i++ {
		v := i
		p.Submit(func() {
			mu.Lock()
			results = append(results, v)
			mu.Unlock()
		})
	}
	p.Wait()
	p.Close()
	if len(results) != 5 {
		t.Errorf("got %d results, want 5", len(results))
	}
}
