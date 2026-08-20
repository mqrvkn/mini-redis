// Package store implements the in-memory data structure that backs the
// server. Week 1 is just a string key-value map. Later weeks will add
// TTLs, lists, and hashes on top of this same locking pattern.
package store

import "sync"

// Store is a thread-safe key-value store.
type Store struct {
	mu   sync.RWMutex
	data map[string]string
}

// New creates an empty store.
func New() *Store {
	return &Store{
		data: make(map[string]string),
	}
}

// Set stores a value for a key, overwriting any existing value.
func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Get retrieves a value. The bool reports whether the key existed.
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// Del removes a key and reports whether it existed.
func (s *Store) Del(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, existed := s.data[key]
	delete(s.data, key)
	return existed
}

// Exists reports whether a key is present.
func (s *Store) Exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[key]
	return ok
}
