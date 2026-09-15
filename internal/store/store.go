// Package store implements the in-memory data structure that backs the
// server.
//
// Week 3 design change: a key used to map directly to a string. Now it
// needs to optionally carry an expiration time, AND hold one of two
// different shapes of data (a plain string, or a list of strings). To
// represent that, every key maps to an *entry — a small wrapper that
// tags what kind of value is stored and, if set, when it expires.
//
// This mirrors how real Redis works internally: every key has a type
// tag, and commands check it before operating (GET on a key that holds
// a list is an error in real Redis too — "WRONGTYPE").
package store

import (
	"errors"
	"sync"
	"time"
)

// ErrWrongType is returned when a command is used against a key that
// holds a different type of value (e.g. LPUSH on a key set with SET).
var ErrWrongType = errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")

// valueType tags what kind of data an entry holds.
type valueType int

const (
	typeString valueType = iota
	typeList
)

// entry is the internal representation of one stored key. Only the
// field matching vtype is meaningful — e.g. for typeList, `list` holds
// the data and `str` is unused.
type entry struct {
	vtype     valueType
	str       string
	list      []string
	expiresAt time.Time // zero value (time.Time{}) means "no expiry"
}

// isExpired reports whether e has a set expiration that has passed.
// A zero expiresAt means no TTL was ever set — never expired.
func (e *entry) isExpired() bool {
	if e.expiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.expiresAt)
}

// Store is a thread-safe key-value store supporting strings, lists,
// and TTL-based expiration.
type Store struct {
	mu   sync.RWMutex
	data map[string]*entry

	stopSweep chan struct{}
}

// New creates an empty store. It does not start the background
// expiration sweeper automatically — call StartSweeper explicitly, so
// that tests which don't need it (e.g. pure list-logic tests) don't pay
// for a goroutine they don't use.
func New() *Store {
	return &Store{
		data: make(map[string]*entry),
	}
}

// --- String commands (Week 1 behavior, now expiry-aware) ---

// Set stores a string value for a key, overwriting any existing value
// (of any type) and clearing any previous expiration.
func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = &entry{vtype: typeString, str: value}
}

// Get retrieves a string value. The bool reports whether the key
// existed, held a string, and was not expired — Get treats an expired
// key the same as a missing one. (It doesn't proactively delete it
// here; that's the sweeper's job, which keeps Get a read-lock-only
// operation. Worth naming as a tradeoff: this means a truly idle,
// never-swept expired key still occupies memory until the sweeper runs
// or something else touches it.)
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	if !ok || e.isExpired() || e.vtype != typeString {
		return "", false
	}
	return e.str, true
}

// Del removes a key (of any type) and reports whether it existed
// (and wasn't already expired).
func (s *Store) Del(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, existed := s.data[key]
	if !existed || e.isExpired() {
		return false
	}
	delete(s.data, key)
	return true
}

// Exists reports whether a key is present, unexpired, of any type.
func (s *Store) Exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	return ok && !e.isExpired()
}

// --- TTL commands ---

// Expire sets key to expire after the given duration from now.
// Returns false if the key doesn't exist (or is already expired).
//
// TODO: implement this.
//
// Steps:
//  1. Lock for writing (this mutates an entry).
//  2. Look up the key. If missing or already expired, return false.
//  3. Otherwise set e.expiresAt = time.Now().Add(ttl) and return true.
func (s *Store) Expire(key string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, existed := s.data[key]
	if !existed || e.isExpired() {
		return false
	}
	e.expiresAt = time.Now().Add(ttl)
	return true
}

// TTL reports how long until key expires, following Redis convention:
//   - returns the remaining duration if key exists and has a TTL set
//   - returns -1 (as a duration you'll need to represent specially —
//     see the (time.Duration, bool) return below) if key exists but
//     has NO expiry set
//   - the bool is false if the key doesn't exist at all (or is expired)
//
// TODO: implement this.
//
// Steps:
//  1. Read-lock, look up the key.
//  2. If missing or expired: return (0, false).
//  3. If found but e.expiresAt.IsZero() (no TTL set): return (-1, true).
//  4. Otherwise: return (time.Until(e.expiresAt), true).
func (s *Store) TTL(key string) (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, existed := s.data[key]
	if !existed || e.isExpired() {
		return 0, false
	}
	if e.expiresAt.IsZero() {
		return -1, true
	}
	return time.Until(e.expiresAt), true
}

// StartSweeper launches a background goroutine that wakes up every
// `interval` and deletes any expired keys. Call StopSweeper to shut it
// down cleanly (important in tests, so goroutines don't leak between
// test cases).
//
// TODO: implement this.
//
// This is the "background goroutine sweeping expired keys" piece from
// the project plan — the concurrency pattern here is: a goroutine on a
// time.Ticker, listening on two channels via select — the ticker's
// channel (do the sweep) and a stop channel (exit cleanly).
//
// Steps:
//  1. Initialize s.stopSweep = make(chan struct{}).
//  2. Launch a goroutine (go func() { ... }()) that:
//     a. creates a time.NewTicker(interval), and defers ticker.Stop()
//     b. loops forever with a `select` on two cases:
//     - <-ticker.C: call s.sweepExpired()
//     - <-s.stopSweep: return (exits the goroutine)
func (s *Store) StartSweeper(interval time.Duration) {
	s.stopSweep = make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.sweepExpired()
			case <-s.stopSweep:
				return
			}
		}
	}()
}

// StopSweeper signals the sweeper goroutine (started by StartSweeper)
// to exit. Safe to call even if StartSweeper was never called, as long
// as you guard against a nil channel.
func (s *Store) StopSweeper() {
	if s.stopSweep != nil {
		close(s.stopSweep)
	}
}

// sweepExpired scans the store and deletes every expired key. Called
// periodically by the sweeper goroutine.
//
// TODO: implement this.
//
// Steps:
//  1. Lock for writing — you're deleting from the map.
//  2. Iterate over s.data (a plain `for key, e := range s.data` loop).
//  3. For each entry where e.isExpired() is true, delete(s.data, key).
//
// Note: it's safe in Go to delete map keys while ranging over that same
// map in the same loop — this is one of the few explicitly-documented
// safe mutation patterns for maps during iteration.
func (s *Store) sweepExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range s.data {
		if value.isExpired() {
			delete(s.data, key)
		}
	}
}

// --- List commands ---

// LPush pushes one or more values onto the front (head) of the list at
// key, creating the list if it doesn't exist. Returns the new length of
// the list.
//
// Real Redis semantics worth matching: `LPUSH key a b c` results in the
// list [c, b, a, ...whatever was already there] — each value in the
// call is pushed to the front in sequence, so the last argument ends up
// closest to the front.
//
// TODO: implement this.
//
// Steps:
//  1. Lock for writing.
//  2. Look up the entry. Three cases:
//     - doesn't exist: create a new entry with vtype: typeList
//     - exists but e.vtype != typeList: return (0, ErrWrongType)
//     - exists and is a list: use it
//  3. For each value in the `values` slice, prepend it to e.list.
//     (Prepending to a slice means building a new slice with the value
//     first, then the old contents after — there's no built-in
//     "prepend," you construct it: append([]string{v}, e.list...))
//  4. Return (len(e.list), nil).
func (s *Store) LPush(key string, values ...string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, existed := s.data[key]
	if !existed {
		e = &entry{vtype: typeList}
		s.data[key] = e

	} else if e.vtype != typeList {
		return 0, ErrWrongType
	}

	for _, v := range values {
		e.list = append([]string{v}, e.list...)
	}
	return len(e.list), nil

}

// RPush pushes one or more values onto the back (tail) of the list at
// key, creating the list if it doesn't exist. Returns the new length.
//
// TODO: implement this. Structurally identical to LPush, except you're
// appending to the end (which IS a direct, cheap `append` in Go — no
// slice-rebuilding needed, unlike LPush's front-insert).
func (s *Store) RPush(key string, values ...string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, existed := s.data[key]
	if !existed {
		e = &entry{vtype: typeList}
		s.data[key] = e

	} else if e.vtype != typeList {
		return 0, ErrWrongType
	}

	for _, v := range values {
		e.list = append(e.list, v)
	}
	return len(e.list), nil
}

// LPop removes and returns the front (head) element of the list at key.
// The bool is false if the key doesn't exist or the list is empty.
//
// TODO: implement this.
//
// Steps:
//  1. Lock for writing.
//  2. Look up the entry. If missing or empty list: return ("", false, nil).
//  3. If wrong type: return ("", false, ErrWrongType).
//  4. Otherwise: take e.list[0], then set e.list = e.list[1:] to drop it.
//  5. If the list is now empty, consider deleting the key entirely
//     (matches real Redis: an empty list key doesn't linger) — your
//     call whether to implement that now or leave a comment about it.
func (s *Store) LPop(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, existed := s.data[key]
	if !existed {
		return "", false, nil

	} else if e.vtype != typeList {
		return "", false, ErrWrongType

	} else if len(e.list) == 0 {
		return "", false, nil

	}

	front := e.list[0]
	e.list = e.list[1:]

	return front, true, nil
}

// RPop removes and returns the back (tail) element of the list at key.
//
// TODO: implement this. Same shape as LPop, but take/remove the LAST
// element: e.list[len(e.list)-1], then truncate with
// e.list = e.list[:len(e.list)-1].
func (s *Store) RPop(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, existed := s.data[key]
	if !existed {
		return "", false, nil

	} else if e.vtype != typeList {
		return "", false, ErrWrongType

	} else if len(e.list) == 0 {
		return "", false, nil

	}

	back := e.list[len(e.list) - 1]
	e.list = e.list[:len(e.list) -1]
	return back, true, nil
}

// LLen returns the number of elements in the list at key (0 if the key
// doesn't exist).
//
// TODO: implement this. Short one — read-lock, look up, return
// len(e.list) or 0, with an ErrWrongType check if it exists as a
// non-list.
func (s *Store) LLen(key string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, existed := s.data[key]
	if !existed {
		return 0, nil
	}
	if e.vtype != typeList {
		return 0, ErrWrongType
	}
	return len(e.list), nil
}

// LRange returns a slice of the list from index start to stop,
// INCLUSIVE on both ends, matching Redis semantics. Supports negative
// indices meaning "from the end": -1 is the last element, -2 is second
// to last, etc.
//
// TODO: implement this — the fiddliest one, save it for last.
//
// Steps:
//  1. Read-lock, look up the entry. Missing key: return (nil, nil) —
//     an empty result, not an error (matches Redis: LRANGE on a
//     nonexistent key returns an empty list, not an error).
//  2. Wrong type: return (nil, ErrWrongType).
//  3. Convert negative indices to positive: if start < 0, start +=
//     len(e.list) (same for stop). Redis clamps out-of-range indices
//     rather than erroring — e.g. a stop far beyond the list length
//     just means "up to the end." Clamp start to 0 minimum, stop to
//     len(e.list)-1 maximum.
//  4. If, after clamping, start > stop (empty range), return an empty
//     slice, not nil and not an error.
//  5. Otherwise return e.list[start : stop+1] (remember Go slice
//     upper bounds are exclusive, hence the +1 to make stop inclusive).
func (s *Store) LRange(key string, start, stop int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, existed := s.data[key]
	if !existed {
		return nil, nil
	
	}
	if e.vtype != typeList {
		return nil, ErrWrongType
	}
	if start < 0 {
		start += len(e.list)

	}
	if stop < 0 {
		stop += len(e.list)
	}
	if start < 0 {
		start = 0

	}
	if stop > len(e.list) - 1 {
		stop = len(e.list) - 1

	}
	if start > stop {
		return []string{}, nil

	}

	return e.list[start : stop + 1], nil

}
