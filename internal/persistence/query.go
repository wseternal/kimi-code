package persistence

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// Query provides filtering and pagination over a Store.
type Query struct {
	store    Store
	prefix   string
	cursor   string // resume after this key
	limit    int
	reverse  bool
}

// NewQuery creates a query builder against the given store.
func NewQuery(store Store) *Query {
	return &Query{store: store, limit: 100}
}

// Prefix filters keys to those starting with the given prefix.
func (q *Query) Prefix(prefix string) *Query {
	q.prefix = prefix
	return q
}

// After sets the cursor — only keys lexicographically after this value are returned.
func (q *Query) After(cursor string) *Query {
	q.cursor = cursor
	return q
}

// Limit caps the number of results. Default 100, max 1000.
func (q *Query) Limit(n int) *Query {
	if n <= 0 {
		n = 100
	}
	if n > 1000 {
		n = 1000
	}
	q.limit = n
	return q
}

// Reverse sorts results in descending key order.
func (q *Query) Reverse() *Query {
	q.reverse = true
	return q
}

// Result holds a key-value pair from a query.
type Result struct {
	Key   string
	Value []byte
}

// Execute runs the query and returns results.
func (q *Query) Execute(ctx context.Context) ([]Result, error) {
	keys, err := q.store.Keys(ctx, q.prefix)
	if err != nil {
		return nil, err
	}

	// Apply cursor filter
	if q.cursor != "" {
		idx := sort.SearchStrings(keys, q.cursor)
		// Skip past the cursor key itself
		for idx < len(keys) && keys[idx] <= q.cursor {
			idx++
		}
		keys = keys[idx:]
	}

	// Apply reverse
	if q.reverse {
		for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
			keys[i], keys[j] = keys[j], keys[i]
		}
	}

	// Apply limit
	if len(keys) > q.limit {
		keys = keys[:q.limit]
	}

	results := make([]Result, 0, len(keys))
	for _, key := range keys {
		data, err := q.store.Get(ctx, key)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue // key deleted between Keys() and Get()
			}
			return nil, err
		}
		results = append(results, Result{Key: key, Value: data})
	}
	return results, nil
}

// AppendLog provides an append-only log on top of a Store.
// Each entry is stored as a separate key with a monotonically increasing sequence number.
type AppendLog struct {
	store  Store
	prefix string
	seq    int
}

// NewAppendLog creates an append log with the given key prefix.
func NewAppendLog(store Store, prefix string) *AppendLog {
	return &AppendLog{store: store, prefix: prefix}
}

// Append adds an entry to the log and returns its sequence number.
func (l *AppendLog) Append(ctx context.Context, data []byte) (int, error) {
	l.seq++
	key := l.keyFor(l.seq)
	if err := l.store.Set(ctx, key, data); err != nil {
		return 0, err
	}
	return l.seq, nil
}

// ReadAll reads all entries from the log in order.
func (l *AppendLog) ReadAll(ctx context.Context) ([]Result, error) {
	return NewQuery(l.store).Prefix(l.prefix).Execute(ctx)
}

func (l *AppendLog) keyFor(seq int) string {
	return l.prefix + "/" + zeroPad(seq, 10)
}

// zeroPad left-pads n with zeros to width w.
func zeroPad(n, w int) string {
	s := strings.Repeat("0", w) + itoa(n)
	return s[len(s)-w:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
