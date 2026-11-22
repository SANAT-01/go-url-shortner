package repository

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache wraps a Redis client to cache short-code -> long-URL lookups, so
// repeat redirects for a popular link skip the Postgres round-trip.
type Cache struct {
	client *redis.Client
}

func NewCache(client *redis.Client) *Cache {
	return &Cache{client: client}
}

// GetURL returns ("", nil) on a cache miss — not an error. redis.Nil is the
// go-redis sentinel for "key doesn't exist", which we translate here so
// callers don't need to know about redis.Nil at all.
func (c *Cache) GetURL(ctx context.Context, code string) (string, error) {
	url, err := c.client.Get(ctx, cacheKey(code)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return url, nil
}

// SetURL caches a code -> URL mapping for 24h. A TTL (time-to-live) is used
// instead of caching forever so that a bad entry (shouldn't happen, since
// links are immutable, but still) eventually self-heals rather than being
// stuck wrong indefinitely.
func (c *Cache) SetURL(ctx context.Context, code, url string) error {
	return c.client.Set(ctx, cacheKey(code), url, 24*time.Hour).Err()
}

// cacheKey namespaces our keys (e.g. "link:abc123") so this app's keys
// don't collide with anything else that might use the same Redis instance.
func cacheKey(code string) string {
	return "link:" + code
}
