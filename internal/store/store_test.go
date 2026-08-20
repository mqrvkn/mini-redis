package store

import (
	"sync"
	"testing"
)

// TestConcurrentSetGet hammers the store with many goroutines doing
// simultaneous writes and reads on overlapping keys. Run with -race:
// if Set/Get/Del/Exists aren't holding the mutex correctly, the race
// detector will flag it even though the test itself won't panic or
// produce wrong values — that's what makes data races insidious.
func TestConcurrentSetGet(t *testing.T) {
	s := New()
	var wg sync.WaitGroup

	const goroutines = 50
	const opsPerGoroutine = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				key := "key"
				s.Set(key, "value")
				s.Get(key)
				s.Exists(key)
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentDistinctKeys is the same idea but each goroutine owns
// its own key, so we can also assert correctness afterward, not just
// "no race detected."
func TestConcurrentDistinctKeys(t *testing.T) {
	s := New()
	var wg sync.WaitGroup

	const goroutines = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "key" + string(rune('A'+id%26))
			s.Set(key, "value")
			s.Get(key)
		}(i)
	}

	wg.Wait()
}

// TestConcurrentSetDel interleaves writers and deleters on the same
// key. This is the pattern most likely to expose a missing lock,
// since Set and Del both mutate the underlying map.
func TestConcurrentSetDel(t *testing.T) {
	s := New()
	var wg sync.WaitGroup

	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				s.Set("shared", "value")
			} else {
				s.Del("shared")
			}
		}(i)
	}

	wg.Wait()
}

// --- Basic correctness tests (not concurrency-focused, but the suite
// should cover normal behavior too) ---

func TestSetAndGet(t *testing.T) {
	s := New()
	s.Set("foo", "bar")

	v, ok := s.Get("foo")
	if !ok {
		t.Fatalf("Get(\"foo\") ok = false, want true")
	}
	if v != "bar" {
		t.Fatalf("Get(\"foo\") = %q, want %q", v, "bar")
	}
}

func TestGetMissingKey(t *testing.T) {
	s := New()
	_, ok := s.Get("missing")
	if ok {
		t.Fatalf("Get(\"missing\") ok = true, want false")
	}
}

func TestDel(t *testing.T) {
	s := New()
	s.Set("foo", "bar")

	if !s.Del("foo") {
		t.Fatalf("Del(\"foo\") = false, want true (key existed)")
	}
	if s.Del("foo") {
		t.Fatalf("Del(\"foo\") second call = true, want false (already gone)")
	}

	_, ok := s.Get("foo")
	if ok {
		t.Fatalf("Get(\"foo\") after Del ok = true, want false")
	}
}

func TestExists(t *testing.T) {
	s := New()
	if s.Exists("foo") {
		t.Fatalf("Exists(\"foo\") = true before Set, want false")
	}
	s.Set("foo", "bar")
	if !s.Exists("foo") {
		t.Fatalf("Exists(\"foo\") = false after Set, want true")
	}
}