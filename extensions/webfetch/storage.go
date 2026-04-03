package webfetch

import (
	"sync"
)

// Storage holds cached fetch/search results for the session.
type Storage struct {
	mu       sync.RWMutex
	fetches  map[string]string        // URL -> content
	searches map[string][]SearchResult // query -> results
}

// NewStorage creates a new session storage.
func NewStorage() *Storage {
	return &Storage{
		fetches:  make(map[string]string),
		searches: make(map[string][]SearchResult),
	}
}

// StoreFetch saves fetch content for later retrieval.
func (s *Storage) StoreFetch(url, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetches[url] = content
}

// GetFetch retrieves cached fetch content. Returns empty string if not found.
func (s *Storage) GetFetch(url string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fetches[url]
}

// StoreSearch saves search results for later retrieval.
func (s *Storage) StoreSearch(query string, results []SearchResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searches[query] = results
}

// GetSearch retrieves cached search results. Returns nil if not found.
func (s *Storage) GetSearch(query string) []SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.searches[query]
}

// List returns all stored URLs and queries.
func (s *Storage) List() (urls, queries []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	urls = make([]string, 0, len(s.fetches))
	for url := range s.fetches {
		urls = append(urls, url)
	}
	queries = make([]string, 0, len(s.searches))
	for q := range s.searches {
		queries = append(queries, q)
	}
	return
}
