package sample

import (
	"fmt"
	"path/filepath"
	"sync"
)

// Cache keeps decoded files in memory so a project that uses one sample on
// twenty clips decodes it once. Safe for concurrent use.
//
// A Cache is bound to a single sample rate — the project's. That is what makes
// the file path alone a sufficient key: every Buffer it hands out is already at
// the right rate, so two callers asking for the same file can share one slice.
// Buffers are never evicted; a project's samples stay resident for its lifetime.
type Cache struct {
	mu   sync.Mutex
	rate int
	bufs map[string]*Buffer // absolute path -> decoded audio
}

// NewCache returns a Cache that decodes everything to rate.
func NewCache(rate int) *Cache {
	return &Cache{
		rate: rate,
		bufs: make(map[string]*Buffer),
	}
}

// Rate is the sample rate every Buffer from this Cache is at.
func (c *Cache) Rate() int { return c.rate }

// Load returns the decoded audio for path, decoding it on first use. The
// returned Buffer is shared with every other caller for the same file, so
// treat it as read-only.
//
// path may be relative; it is resolved against the process working directory so
// that "samples/kick.wav" and "./samples/kick.wav" are one cache entry and not
// two copies of the same audio.
func (c *Cache) Load(path string) (*Buffer, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("sample: resolve %s: %w", path, err)
	}

	// The lock is held across the decode, so two goroutines asking for
	// different files queue up behind each other. That is fine: the cache is
	// only ever touched at graph-build time, never while mixing. Releasing it
	// during the decode would mean tracking in-flight loads to stop the same
	// file being decoded twice, which is more machinery than the contention is
	// worth.
	c.mu.Lock()
	defer c.mu.Unlock()

	if b, ok := c.bufs[abs]; ok {
		return b, nil
	}

	b, err := Load(abs, c.rate)
	if err != nil {
		return nil, err
	}
	c.bufs[abs] = b
	return b, nil
}
