// Package recall provides cross-session context search via a TF-IDF inverted index.
package recall

import (
	"bytes"
	"cmp"
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

// Index is a TF-IDF inverted index over session text.
type Index struct {
	mu        sync.RWMutex
	DocFreq   map[string]int       // term → number of documents containing it
	Docs      map[string]*DocEntry // session ID → document
	TotalDocs int
	MaxDocs   int // cap; prune oldest when exceeded (default 500)
}

// DocEntry holds the indexed data for a single session.
type DocEntry struct {
	SessionID  string
	Path       string
	Title      string
	TermFreqs  map[string]int
	TotalTerms int
	IndexedAt  time.Time
}

// SearchResult is a ranked result from a search query.
type SearchResult struct {
	SessionID string
	Path      string
	Title     string
	Score     float64
}

// indexData is the serializable form of Index (no mutex).
type indexData struct {
	Docs      map[string]*DocEntry
	TotalDocs int
	MaxDocs   int
}

// NewIndex creates an empty index with the given document cap.
func NewIndex(maxDocs int) *Index {
	if maxDocs <= 0 {
		maxDocs = 500
	}
	return &Index{
		DocFreq: make(map[string]int),
		Docs:    make(map[string]*DocEntry),
		MaxDocs: maxDocs,
	}
}

// LoadIndex deserializes an index from path.
// Returns a fresh index on any error (missing file, corrupt data).
func LoadIndex(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read index %s: %w", path, err)
	}

	var d indexData
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}

	idx := &Index{
		DocFreq:   make(map[string]int),
		Docs:      d.Docs,
		TotalDocs: d.TotalDocs,
		MaxDocs:   d.MaxDocs,
	}
	if idx.Docs == nil {
		idx.Docs = make(map[string]*DocEntry)
	}

	// Rebuild DocFreq from loaded documents.
	for _, doc := range idx.Docs {
		for term := range doc.TermFreqs {
			idx.DocFreq[term]++
		}
	}

	return idx, nil
}

// Save serializes the index to path atomically.
func (idx *Index) Save(path string) error {
	idx.mu.RLock()
	d := indexData{
		Docs:      idx.Docs,
		TotalDocs: idx.TotalDocs,
		MaxDocs:   idx.MaxDocs,
	}
	idx.mu.RUnlock()

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(d); err != nil {
		return fmt.Errorf("encode index: %w", err)
	}
	return xdg.WriteFileAtomic(path, buf.Bytes())
}

// AddDocument tokenizes text and adds it to the index under sessionID.
// If MaxDocs is exceeded, the oldest document (by IndexedAt) is pruned.
func (idx *Index) AddDocument(sessionID, path, title, text string) {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return
	}

	freqs := make(map[string]int, len(tokens))
	for _, t := range tokens {
		freqs[t]++
	}

	entry := &DocEntry{
		SessionID:  sessionID,
		Path:       path,
		Title:      title,
		TermFreqs:  freqs,
		TotalTerms: len(tokens),
		IndexedAt:  time.Now(),
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove old entry for this session if it exists.
	if old, ok := idx.Docs[sessionID]; ok {
		for term := range old.TermFreqs {
			idx.DocFreq[term]--
			if idx.DocFreq[term] <= 0 {
				delete(idx.DocFreq, term)
			}
		}
		idx.TotalDocs--
	}

	idx.Docs[sessionID] = entry
	idx.TotalDocs++

	for term := range freqs {
		idx.DocFreq[term]++
	}

	idx.pruneOldest()
}

// pruneOldest removes the oldest document when MaxDocs is exceeded.
// Caller must hold write lock.
func (idx *Index) pruneOldest() {
	for len(idx.Docs) > idx.MaxDocs {
		var oldestID string
		var oldestTime time.Time

		for id, doc := range idx.Docs {
			if oldestID == "" || doc.IndexedAt.Before(oldestTime) {
				oldestID = id
				oldestTime = doc.IndexedAt
			}
		}
		if oldestID == "" {
			break
		}
		idx.removeDoc(oldestID)
	}
}

// removeDoc removes a document and updates DocFreq.
// Caller must hold write lock.
func (idx *Index) removeDoc(sessionID string) {
	doc, ok := idx.Docs[sessionID]
	if !ok {
		return
	}
	for term := range doc.TermFreqs {
		idx.DocFreq[term]--
		if idx.DocFreq[term] <= 0 {
			delete(idx.DocFreq, term)
		}
	}
	delete(idx.Docs, sessionID)
	idx.TotalDocs--
}

// Remove deletes a document from the index and updates doc frequencies.
func (idx *Index) Remove(sessionID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeDoc(sessionID)
}

// Search tokenizes query, scores each document by TF-IDF, and returns the
// top limit results sorted by descending score.
func (idx *Index) Search(query string, limit int) []SearchResult {
	if limit <= 0 {
		limit = 3
	}

	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.TotalDocs == 0 {
		return nil
	}

	scores := make(map[string]float64, len(idx.Docs))
	for _, term := range terms {
		df, ok := idx.DocFreq[term]
		if !ok {
			continue
		}
		idf := math.Log(1 + float64(idx.TotalDocs)/float64(1+df))

		for id, doc := range idx.Docs {
			freq, ok := doc.TermFreqs[term]
			if !ok || doc.TotalTerms == 0 {
				continue
			}
			tf := float64(freq) / float64(doc.TotalTerms)
			scores[id] += tf * idf
		}
	}

	results := make([]SearchResult, 0, len(scores))
	for id, score := range scores {
		if score <= 0 {
			continue
		}
		doc := idx.Docs[id]
		results = append(results, SearchResult{
			SessionID: id,
			Path:      doc.Path,
			Title:     doc.Title,
			Score:     score,
		})
	}

	slices.SortFunc(results, func(a, b SearchResult) int {
		return cmp.Compare(b.Score, a.Score)
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// Stats returns the number of indexed documents and unique terms.
func (idx *Index) Stats() (docCount, termCount int) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.TotalDocs, len(idx.DocFreq)
}

// stopwords is the set of terms excluded during tokenization.
var stopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"in": {}, "on": {}, "at": {}, "to": {}, "for": {}, "of": {}, "and": {},
	"or": {}, "but": {}, "not": {}, "with": {}, "this": {}, "that": {},
	"it": {}, "be": {}, "as": {}, "by": {}, "from": {},
}

// tokenize lowercases text, splits on non-alphanumeric characters, removes
// stopwords, deduplicates, and filters tokens shorter than 2 characters.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) < 2 {
			continue
		}
		if _, stop := stopwords[p]; stop {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
	}
	return result
}
