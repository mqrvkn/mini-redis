package store

import (
	"sync"
	"testing"
	"time"
)

// ============================================================
// Week 1 tests (unchanged behavior — should already pass, and
// confirm your entry-based rewrite didn't break basic get/set)
// ============================================================

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
				s.Set("key", "value")
				s.Get("key")
				s.Exists("key")
			}
		}(i)
	}

	wg.Wait()
}

// ============================================================
// Week 3: TTL tests
// ============================================================

func TestExpireAndTTL(t *testing.T) {
	s := New()
	s.Set("foo", "bar")

	if !s.Expire("foo", 50*time.Millisecond) {
		t.Fatalf("Expire on existing key = false, want true")
	}

	ttl, ok := s.TTL("foo")
	if !ok {
		t.Fatalf("TTL ok = false right after Expire, want true")
	}
	if ttl <= 0 || ttl > 50*time.Millisecond {
		t.Fatalf("TTL = %v, want something between 0 and 50ms", ttl)
	}
}

func TestExpireMissingKey(t *testing.T) {
	s := New()
	if s.Expire("nope", time.Second) {
		t.Fatalf("Expire on missing key = true, want false")
	}
}

func TestTTLNoExpirySet(t *testing.T) {
	s := New()
	s.Set("foo", "bar")

	ttl, ok := s.TTL("foo")
	if !ok {
		t.Fatalf("TTL ok = false, want true (key exists)")
	}
	if ttl != -1 {
		t.Fatalf("TTL = %v, want -1 (no expiry set)", ttl)
	}
}

func TestTTLMissingKey(t *testing.T) {
	s := New()
	_, ok := s.TTL("nope")
	if ok {
		t.Fatalf("TTL ok = true for missing key, want false")
	}
}

// This is the key correctness test: a key with a short TTL should
// actually become inaccessible via Get once time passes, even WITHOUT
// the sweeper running — this is "lazy expiration," and it must work on
// its own before the sweeper is even involved.
func TestKeyExpiresLazily(t *testing.T) {
	s := New()
	s.Set("foo", "bar")
	s.Expire("foo", 20*time.Millisecond)

	if _, ok := s.Get("foo"); !ok {
		t.Fatalf("Get(\"foo\") immediately after Expire = false, want true (not expired yet)")
	}

	time.Sleep(40 * time.Millisecond)

	if _, ok := s.Get("foo"); ok {
		t.Fatalf("Get(\"foo\") after TTL passed = true, want false (should be treated as expired)")
	}
	if s.Exists("foo") {
		t.Fatalf("Exists(\"foo\") after TTL passed = true, want false")
	}
}

// This test proves the SWEEPER specifically — not just lazy expiration
// — actually removes expired keys from the underlying map, by checking
// that Del (which reports whether a key existed) returns false after
// the sweeper interval passes, for a key nobody ever read.
func TestSweeperRemovesExpiredKeys(t *testing.T) {
	s := New()
	s.StartSweeper(10 * time.Millisecond)
	defer s.StopSweeper()

	s.Set("foo", "bar")
	s.Expire("foo", 15*time.Millisecond)

	// Give the sweeper time to run at least once after expiry.
	time.Sleep(60 * time.Millisecond)

	if s.Del("foo") {
		t.Fatalf("Del(\"foo\") after sweeper should have run = true, want false (sweeper should have already removed it)")
	}
}

func TestStopSweeperWithoutStart(t *testing.T) {
	s := New()
	// Should not panic even though StartSweeper was never called.
	s.StopSweeper()
}

// ============================================================
// Week 3: List tests
// ============================================================

func TestLPushAndLLen(t *testing.T) {
	s := New()
	n, err := s.LPush("mylist", "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("LPush length = %d, want 1", n)
	}

	length, err := s.LLen("mylist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if length != 1 {
		t.Fatalf("LLen = %d, want 1", length)
	}
}

// LPUSH key a b c should result in [c, b, a] — each pushed value lands
// at the front, so later arguments end up closer to the head.
func TestLPushOrder(t *testing.T) {
	s := New()
	if _, err := s.LPush("mylist", "a", "b", "c"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := s.LRange("mylist", 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"c", "b", "a"}
	if !equalSlices(got, want) {
		t.Fatalf("LRange = %v, want %v", got, want)
	}
}

func TestRPushOrder(t *testing.T) {
	s := New()
	if _, err := s.RPush("mylist", "a", "b", "c"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := s.LRange("mylist", 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !equalSlices(got, want) {
		t.Fatalf("LRange = %v, want %v", got, want)
	}
}

func TestLPopRPop(t *testing.T) {
	s := New()
	s.RPush("mylist", "a", "b", "c") // [a, b, c]

	front, ok, err := s.LPop("mylist")
	if err != nil || !ok || front != "a" {
		t.Fatalf("LPop = (%q, %v, %v), want (\"a\", true, nil)", front, ok, err)
	}

	back, ok, err := s.RPop("mylist")
	if err != nil || !ok || back != "c" {
		t.Fatalf("RPop = (%q, %v, %v), want (\"c\", true, nil)", back, ok, err)
	}

	length, _ := s.LLen("mylist")
	if length != 1 {
		t.Fatalf("LLen after two pops = %d, want 1", length)
	}
}

func TestLPopEmptyOrMissing(t *testing.T) {
	s := New()
	_, ok, err := s.LPop("nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("LPop on missing key ok = true, want false")
	}
}

func TestLRangeNegativeIndices(t *testing.T) {
	s := New()
	s.RPush("mylist", "a", "b", "c", "d", "e") // [a,b,c,d,e]

	got, err := s.LRange("mylist", -2, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"d", "e"}
	if !equalSlices(got, want) {
		t.Fatalf("LRange(-2, -1) = %v, want %v", got, want)
	}
}

func TestLRangeOutOfBoundsClamps(t *testing.T) {
	s := New()
	s.RPush("mylist", "a", "b", "c")

	got, err := s.LRange("mylist", 0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !equalSlices(got, want) {
		t.Fatalf("LRange(0, 100) = %v, want %v (should clamp, not error)", got, want)
	}
}

func TestLRangeOnMissingKey(t *testing.T) {
	s := New()
	got, err := s.LRange("nope", 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("LRange on missing key = %v, want empty", got)
	}
}

func TestWrongTypeErrors(t *testing.T) {
	s := New()
	s.Set("strkey", "hello")

	if _, err := s.LPush("strkey", "x"); err != ErrWrongType {
		t.Fatalf("LPush on string key error = %v, want ErrWrongType", err)
	}

	s.RPush("listkey", "x")
	if _, ok := s.Get("listkey"); ok {
		t.Fatalf("Get on list key ok = true, want false (wrong type)")
	}
}

func TestConcurrentListPushPop(t *testing.T) {
	s := New()
	var wg sync.WaitGroup

	const goroutines = 50
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.RPush("shared", "x")
			s.LPop("shared")
		}()
	}
	wg.Wait()
}

// --- test helper ---

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
