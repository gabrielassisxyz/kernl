package backend

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SuppressionWindow = 2 * time.Minute
	CacheTTL          = 10 * time.Minute
	MaxCacheEntries   = 64
)

var suppressiblePatterns = []string{
	"lock",
	"locked",
	"timed out waiting for bd repo lock",
	"bd command timed out",
	"database is locked",
	"unable to open database",
	"could not obtain lock",
	"busy",
	"eacces",
	"permission denied",
}

const DegradedErrorMessage = "Unable to interact with beads store, try refreshing the page or restarting Kernl. If problems persist, investigate your beads install"

func IsSuppressibleError(msg string) bool {
	lower := strings.ToLower(msg)
	for _, p := range suppressiblePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

type resultCacheEntry struct {
	data      []Bead
	timestamp time.Time
}

type failureState struct {
	firstFailedAt time.Time
}

type ErrorSuppressionCache struct {
	mu           sync.RWMutex
	resultCache  map[string]*resultCacheEntry
	failureState map[string]*failureState
	window       time.Duration
	ttl          time.Duration
	maxEntries   int
	nowFunc      func() time.Time
}

func NewErrorSuppressionCache() *ErrorSuppressionCache {
	return &ErrorSuppressionCache{
		resultCache:  make(map[string]*resultCacheEntry),
		failureState: make(map[string]*failureState),
		window:       SuppressionWindow,
		ttl:          CacheTTL,
		maxEntries:   MaxCacheEntries,
		nowFunc:      time.Now,
	}
}

func (c *ErrorSuppressionCache) CacheKey(fn string, filters map[string]string, repoPath string, query string) string {
	sorted := "{}"
	if len(filters) > 0 {
		keys := make([]string, 0, len(filters))
		for k := range filters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, fmt.Sprintf("%q:%q", k, filters[k]))
		}
		sorted = "{" + strings.Join(pairs, ",") + "}"
	}
	return fn + ":" + query + ":" + sorted + ":" + repoPath
}

func (c *ErrorSuppressionCache) WithErrorSuppression(fn string, result BackendResult[[]Bead], filters map[string]string, repoPath string, query string) BackendResult[[]Bead] {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.CacheKey(fn, filters, repoPath, query)
	now := c.nowFunc()

	if result.OK {
		c.resultCache[key] = &resultCacheEntry{data: *result.Data, timestamp: now}
		c.evictIfNeeded()
		delete(c.failureState, key)
		return result
	}

	errMsg := ""
	if result.Error != nil {
		errMsg = result.Error.Message
	}

	if !IsSuppressibleError(errMsg) {
		return result
	}

	failure, hasFailure := c.failureState[key]

	if hasFailure && now.Sub(failure.firstFailedAt) >= c.window {
		delete(c.resultCache, key)
		return ErrResult[[]Bead](NewBackendError(ErrorCodeUnavailable, DegradedErrorMessage, WithRetryable(true)))
	}

	cached, hasCached := c.resultCache[key]
	if hasCached {
		if now.Sub(cached.timestamp) > c.ttl {
			delete(c.resultCache, key)
			delete(c.failureState, key)
			return result
		}
	}

	if !hasCached {
		if !hasFailure {
			c.failureState[key] = &failureState{firstFailedAt: now}
		}
		return ErrResult[[]Bead](NewBackendError(ErrorCodeUnavailable, DegradedErrorMessage, WithRetryable(true)))
	}

	if !hasFailure {
		c.failureState[key] = &failureState{firstFailedAt: now}
	}

	beads := cached.data
	return OkResult(beads)
}

func (c *ErrorSuppressionCache) evictIfNeeded() {
	if len(c.resultCache) <= c.maxEntries {
		return
	}
	oldestKey := ""
	oldestTs := time.Time{}
	for k, entry := range c.resultCache {
		if oldestKey == "" || entry.timestamp.Before(oldestTs) {
			oldestTs = entry.timestamp
			oldestKey = k
		}
	}
	if oldestKey != "" {
		delete(c.resultCache, oldestKey)
		delete(c.failureState, oldestKey)
	}
}

func (c *ErrorSuppressionCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resultCache = make(map[string]*resultCacheEntry)
	c.failureState = make(map[string]*failureState)
}

func (c *ErrorSuppressionCache) Internals() *SuppressionInternals {
	return &SuppressionInternals{cache: c}
}

type SuppressionInternals struct {
	cache *ErrorSuppressionCache
}

func (i *SuppressionInternals) ResultCache() map[string]*resultCacheEntry {
	i.cache.mu.RLock()
	defer i.cache.mu.RUnlock()
	cp := make(map[string]*resultCacheEntry, len(i.cache.resultCache))
	for k, v := range i.cache.resultCache {
		entry := *v
		cp[k] = &entry
	}
	return cp
}

func (i *SuppressionInternals) FailureState() map[string]*failureState {
	i.cache.mu.RLock()
	defer i.cache.mu.RUnlock()
	cp := make(map[string]*failureState, len(i.cache.failureState))
	for k, v := range i.cache.failureState {
		entry := *v
		cp[k] = &entry
	}
	return cp
}

func (i *SuppressionInternals) SetResultCacheTimestamp(key string, ts time.Time) {
	i.cache.mu.Lock()
	defer i.cache.mu.Unlock()
	if entry, ok := i.cache.resultCache[key]; ok {
		entry.timestamp = ts
	}
}

func (i *SuppressionInternals) SetFailureFirstFailedAt(key string, ts time.Time) {
	i.cache.mu.Lock()
	defer i.cache.mu.Unlock()
	if entry, ok := i.cache.failureState[key]; ok {
		entry.firstFailedAt = ts
	}
}

func suppressJSON(v map[string]string) string {
	if v == nil {
		return "{}"
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b, _ := json.Marshal(keys)
	return string(b)
}
