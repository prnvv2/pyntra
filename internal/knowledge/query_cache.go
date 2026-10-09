package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// queryEmbeddingCache is a small, bounded, process-wide cache of text->embedding
// for repeated queries. It avoids re-embedding identical queries (a common case
// when an agent retries or refines). Bounded size keeps memory in check; on
// overflow the cache is cleared wholesale (simple and allocation-cheap).
type queryEmbeddingCache struct {
	mu    sync.RWMutex
	m     map[string][]float32
	limit int
}

var qCache = &queryEmbeddingCache{m: make(map[string][]float32), limit: 1024}

func cacheKey(model, text string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(sum[:])
}

func (c *queryEmbeddingCache) get(key string) ([]float32, bool) {
	c.mu.RLock()
	v, ok := c.m[key]
	c.mu.RUnlock()
	return v, ok
}

func (c *queryEmbeddingCache) put(key string, vec []float32) {
	c.mu.Lock()
	if len(c.m) >= c.limit {
		c.m = make(map[string][]float32, c.limit)
	}
	c.m[key] = vec
	c.mu.Unlock()
}
