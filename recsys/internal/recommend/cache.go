package recommend

import (
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

const (
	onlineCacheTTL   = 30 * time.Second
	onlineCacheSlots = 512
)

type onlineKey struct {
	snapshot string
	group    domain.ClientGroup
	kind     string
	industry string
	segment  string
	items    [sha256.Size]byte
	limit    int
}

func recommendationKey(snapshot string, client domain.Client, items []domain.ItemID, limit int) onlineKey {
	history, _ := json.Marshal(items) // ItemID is a string.
	key := onlineKey{snapshot: snapshot, group: client.Group, kind: client.Type, items: sha256.Sum256(history), limit: limit}
	if client.Industry != nil {
		key.industry = *client.Industry
	}
	if client.Segment != nil {
		key.segment = *client.Segment
	}
	return key
}

type onlineCacheEntry struct {
	payload []byte
	expires time.Time
}

type onlineCache struct {
	mu      sync.Mutex
	entries map[onlineKey]onlineCacheEntry
}

func (c *onlineCache) load(key onlineKey) (contract.RecommendationResult, bool) {
	c.mu.Lock()
	entry, found := c.entries[key]
	if found && !time.Now().Before(entry.expires) {
		delete(c.entries, key)
		found = false
	}
	c.mu.Unlock()

	var result contract.RecommendationResult
	if !found {
		return result, false
	}
	err := json.Unmarshal(entry.payload, &result)
	return result, err == nil
}

func (c *onlineCache) save(key onlineKey, result contract.RecommendationResult) {
	payload, err := json.Marshal(result)
	if err != nil {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[onlineKey]onlineCacheEntry)
	}
	if len(c.entries) >= onlineCacheSlots {
		var oldestKey onlineKey
		var oldest time.Time
		for candidate, entry := range c.entries {
			if !now.Before(entry.expires) {
				delete(c.entries, candidate)
			} else if oldest.IsZero() || entry.expires.Before(oldest) {
				oldestKey, oldest = candidate, entry.expires
			}
		}
		if len(c.entries) >= onlineCacheSlots {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = onlineCacheEntry{payload: payload, expires: now.Add(onlineCacheTTL)}
}
