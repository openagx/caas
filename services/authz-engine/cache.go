package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides permission check caching.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a Redis connection for caching.
func NewRedisCache(url string) (*RedisCache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &RedisCache{client: client}, nil
}

// cacheKey builds a deterministic key for a permission check.
func cacheKey(resourceType, resourceID, permission, subjectType, subjectID string) string {
	return fmt.Sprintf("authz:%s:%s#%s@%s:%s", resourceType, resourceID, permission, subjectType, subjectID)
}

// GetCachedCheck returns a cached permission result, or false if not cached.
func (c *RedisCache) GetCachedCheck(ctx context.Context, resourceType, resourceID, permission, subjectType, subjectID string) (allowed bool, found bool) {
	if c == nil {
		return false, false
	}
	key := cacheKey(resourceType, resourceID, permission, subjectType, subjectID)
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return false, false
	}
	return val == "1", true
}

// SetCachedCheck stores a permission result in cache with short TTL.
// Short TTL (5s) balances performance with consistency.
func (c *RedisCache) SetCachedCheck(ctx context.Context, resourceType, resourceID, permission, subjectType, subjectID string, allowed bool) {
	if c == nil {
		return
	}
	key := cacheKey(resourceType, resourceID, permission, subjectType, subjectID)
	val := "0"
	if allowed {
		val = "1"
	}
	c.client.Set(ctx, key, val, 5*time.Second)
}

// InvalidateEntity removes all cached checks for an entity.
func (c *RedisCache) InvalidateEntity(ctx context.Context, entityType, entityID string) {
	if c == nil {
		return
	}
	// Scan and delete keys matching this entity as subject
	pattern := fmt.Sprintf("authz:*@%s:%s", entityType, entityID)
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		c.client.Del(ctx, iter.Val())
	}
}
