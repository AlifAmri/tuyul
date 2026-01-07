package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps redis client with helper methods
type Client struct {
	client *redis.Client
}

// Config holds Redis configuration
type Config struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// New creates a new Redis client
func New(cfg Config) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Client{client: rdb}, nil
}

// GetClient returns the underlying redis client
func (c *Client) GetClient() *redis.Client {
	return c.client
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.client.Close()
}

// Ping tests the connection
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Set sets a key-value pair with optional expiration
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.client.Set(ctx, key, value, expiration).Err()
}

// Get gets a value by key
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Del deletes one or more keys
func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	result, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// HSet sets a field in a hash
func (c *Client) HSet(ctx context.Context, key string, values ...interface{}) error {
	return c.client.HSet(ctx, key, values...).Err()
}

// HGet gets a field from a hash
func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	return c.client.HGet(ctx, key, field).Result()
}

// HGetAll gets all fields and values from a hash
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

// HDel deletes fields from a hash
func (c *Client) HDel(ctx context.Context, key string, fields ...string) error {
	return c.client.HDel(ctx, key, fields...).Err()
}

// HExists checks if a field exists in a hash
func (c *Client) HExists(ctx context.Context, key, field string) (bool, error) {
	return c.client.HExists(ctx, key, field).Result()
}

// ZAdd adds members to a sorted set
func (c *Client) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	return c.client.ZAdd(ctx, key, members...).Err()
}

// ZRange gets members from a sorted set by range
func (c *Client) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.ZRange(ctx, key, start, stop).Result()
}

// ZRevRange gets members from a sorted set by range (descending)
func (c *Client) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.ZRevRange(ctx, key, start, stop).Result()
}

// ZRangeWithScores gets members with scores from a sorted set
func (c *Client) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return c.client.ZRangeWithScores(ctx, key, start, stop).Result()
}

// ZRevRangeWithScores gets members with scores from a sorted set (descending)
func (c *Client) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return c.client.ZRevRangeWithScores(ctx, key, start, stop).Result()
}

// ZRem removes members from a sorted set
func (c *Client) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.ZRem(ctx, key, members...).Err()
}

// ZScore gets the score of a member in a sorted set
func (c *Client) ZScore(ctx context.Context, key, member string) (float64, error) {
	return c.client.ZScore(ctx, key, member).Result()
}

// SAdd adds members to a set
func (c *Client) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SAdd(ctx, key, members...).Err()
}

// SMembers gets all members of a set
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

// SRem removes members from a set
func (c *Client) SRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SRem(ctx, key, members...).Err()
}

// SIsMember checks if a value is a member of a set
func (c *Client) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return c.client.SIsMember(ctx, key, member).Result()
}

// Expire sets an expiration on a key
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.client.Expire(ctx, key, expiration).Err()
}

// TTL gets the time to live for a key
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

// Pipeline creates a new pipeline
func (c *Client) Pipeline() redis.Pipeliner {
	return c.client.Pipeline()
}

// TxPipeline creates a new transaction pipeline
func (c *Client) TxPipeline() redis.Pipeliner {
	return c.client.TxPipeline()
}

// Publish publishes a message to a channel
func (c *Client) Publish(ctx context.Context, channel string, message interface{}) error {
	return c.client.Publish(ctx, channel, message).Err()
}

// Subscribe subscribes to one or more channels
func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.client.Subscribe(ctx, channels...)
}

// Keys returns all keys matching pattern
func (c *Client) Keys(ctx context.Context, pattern string) ([]string, error) {
	return c.client.Keys(ctx, pattern).Result()
}

// Scan iterates over keys
func (c *Client) Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return c.client.Scan(ctx, cursor, match, count).Result()
}

// Incr increments a key by 1
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, key).Result()
}

// IncrBy increments a key by a value
func (c *Client) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.client.IncrBy(ctx, key, value).Result()
}

// Decr decrements a key by 1
func (c *Client) Decr(ctx context.Context, key string) (int64, error) {
	return c.client.Decr(ctx, key).Result()
}

// DecrBy decrements a key by a value
func (c *Client) DecrBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.client.DecrBy(ctx, key, value).Result()
}

