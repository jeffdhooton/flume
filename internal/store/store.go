// Package store provides a BadgerDB-backed ring buffer for captured requests.
package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Store wraps BadgerDB with TTL-based expiry and max-entry enforcement.
type Store struct {
	db          *badger.DB
	ttl         time.Duration
	maxEntries  int
	stopGC      chan struct{}
}

// Options configures the store.
type Options struct {
	Dir        string
	TTL        time.Duration
	MaxEntries int
}

// DefaultOptions returns sane defaults.
func DefaultOptions() Options {
	return Options{
		TTL:        30 * time.Minute,
		MaxEntries: 1000,
	}
}

// Open creates or opens a store at opts.Dir.
func Open(opts Options) (*Store, error) {
	if opts.Dir == "" {
		return nil, fmt.Errorf("store dir is required")
	}
	if opts.TTL == 0 {
		opts.TTL = DefaultOptions().TTL
	}
	if opts.MaxEntries == 0 {
		opts.MaxEntries = DefaultOptions().MaxEntries
	}

	bOpts := badger.DefaultOptions(opts.Dir).
		WithLogger(nil). // suppress badger's own logging
		WithValueLogFileSize(64 << 20)
	db, err := badger.Open(bOpts)
	if err != nil {
		return nil, fmt.Errorf("open badger: %w", err)
	}

	s := &Store{
		db:         db,
		ttl:        opts.TTL,
		maxEntries: opts.MaxEntries,
		stopGC:     make(chan struct{}),
	}
	go s.gcLoop()
	return s, nil
}

// Put stores a captured request with TTL.
func (s *Store) Put(req *Request) error {
	val, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Key: req:<timestamp_ns>:<id>
	key := fmt.Sprintf("req:%020d:%s", req.StartedAt.UnixNano(), req.ID)

	return s.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry([]byte(key), val).WithTTL(s.ttl)
		return txn.SetEntry(entry)
	})
}

// List returns recent requests matching the filter, newest first.
func (s *Store) List(f ListFilter) ([]RequestSummary, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	var results []RequestSummary

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true
		opts.PrefetchValues = true
		opts.Prefix = []byte("req:")

		it := txn.NewIterator(opts)
		defer it.Close()

		// Seek to the end of the prefix range for reverse iteration.
		it.Seek([]byte("req:~"))

		for it.Valid() {
			if len(results) >= limit {
				break
			}
			item := it.Item()
			var req Request
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &req)
			})
			if err != nil {
				it.Next()
				continue
			}

			if matchesFilter(&req, &f) {
				results = append(results, req.Summary())
			}
			it.Next()
		}
		return nil
	})
	return results, err
}

// Get retrieves a single request by ID.
func (s *Store) Get(id string) (*Request, error) {
	var req *Request

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		opts.Prefix = []byte("req:")

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte("req:")); it.Valid(); it.Next() {
			// Check if key ends with the ID.
			key := string(it.Item().Key())
			if !strings.HasSuffix(key, ":"+id) {
				continue
			}
			return it.Item().Value(func(val []byte) error {
				req = new(Request)
				return json.Unmarshal(val, req)
			})
		}
		return fmt.Errorf("request %q not found", id)
	})
	return req, err
}

// Count returns the number of stored requests.
func (s *Store) Count() int {
	count := 0
	_ = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = []byte("req:")
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek([]byte("req:")); it.Valid(); it.Next() {
			count++
		}
		return nil
	})
	return count
}

// Close shuts down the store.
func (s *Store) Close() error {
	close(s.stopGC)
	return s.db.Close()
}

func (s *Store) gcLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.db.RunValueLogGC(0.5)
		case <-s.stopGC:
			return
		}
	}
}

func matchesFilter(req *Request, f *ListFilter) bool {
	if f.Method != "" && !strings.EqualFold(req.Method, f.Method) {
		return false
	}
	if f.Path != "" && !strings.Contains(req.Path, f.Path) {
		return false
	}
	if f.StatusMin > 0 && req.StatusCode < f.StatusMin {
		return false
	}
	if f.StatusMax > 0 && req.StatusCode > f.StatusMax {
		return false
	}
	return true
}
