package redis

import (
	"context"
	"encoding/json"
	"time"
)

// SetJSON sets a key with JSON-encoded value
func (c *Client) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, data, expiration)
}

// GetJSON gets a key and decodes JSON value
func (c *Client) GetJSON(ctx context.Context, key string, dest interface{}) error {
	data, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

// HSetJSON sets a hash field with JSON-encoded value
func (c *Client) HSetJSON(ctx context.Context, key, field string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.HSet(ctx, key, field, data)
}

// HGetJSON gets a hash field and decodes JSON value
func (c *Client) HGetJSON(ctx context.Context, key, field string, dest interface{}) error {
	data, err := c.HGet(ctx, key, field)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

// SetNX sets a key only if it does not exist
func (c *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	return c.client.SetNX(ctx, key, value, expiration).Result()
}

// GetDel gets and deletes a key atomically
func (c *Client) GetDel(ctx context.Context, key string) (string, error) {
	return c.client.GetDel(ctx, key).Result()
}

// MGet gets multiple keys at once
func (c *Client) MGet(ctx context.Context, keys ...string) ([]interface{}, error) {
	return c.client.MGet(ctx, keys...).Result()
}

// MSet sets multiple key-value pairs at once
func (c *Client) MSet(ctx context.Context, pairs ...interface{}) error {
	return c.client.MSet(ctx, pairs...).Err()
}

// IncrByFloat increments a key by a float value
func (c *Client) IncrByFloat(ctx context.Context, key string, value float64) (float64, error) {
	return c.client.IncrByFloat(ctx, key, value).Result()
}

// SCard gets the number of members in a set
func (c *Client) SCard(ctx context.Context, key string) (int64, error) {
	return c.client.SCard(ctx, key).Result()
}

// ZCard gets the number of members in a sorted set
func (c *Client) ZCard(ctx context.Context, key string) (int64, error) {
	return c.client.ZCard(ctx, key).Result()
}

// ZCount counts members in a sorted set with scores between min and max
func (c *Client) ZCount(ctx context.Context, key, min, max string) (int64, error) {
	return c.client.ZCount(ctx, key, min, max).Result()
}

// ZIncrBy increments the score of a member in a sorted set
func (c *Client) ZIncrBy(ctx context.Context, key string, increment float64, member string) (float64, error) {
	return c.client.ZIncrBy(ctx, key, increment, member).Result()
}

// ZRank gets the rank of a member in a sorted set
func (c *Client) ZRank(ctx context.Context, key, member string) (int64, error) {
	return c.client.ZRank(ctx, key, member).Result()
}

// ZRevRank gets the rank of a member in a sorted set (descending)
func (c *Client) ZRevRank(ctx context.Context, key, member string) (int64, error) {
	return c.client.ZRevRank(ctx, key, member).Result()
}

// HLen gets the number of fields in a hash
func (c *Client) HLen(ctx context.Context, key string) (int64, error) {
	return c.client.HLen(ctx, key).Result()
}

// HKeys gets all field names in a hash
func (c *Client) HKeys(ctx context.Context, key string) ([]string, error) {
	return c.client.HKeys(ctx, key).Result()
}

// HVals gets all values in a hash
func (c *Client) HVals(ctx context.Context, key string) ([]string, error) {
	return c.client.HVals(ctx, key).Result()
}

// HIncrBy increments a hash field by an integer
func (c *Client) HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error) {
	return c.client.HIncrBy(ctx, key, field, incr).Result()
}

// HIncrByFloat increments a hash field by a float
func (c *Client) HIncrByFloat(ctx context.Context, key, field string, incr float64) (float64, error) {
	return c.client.HIncrByFloat(ctx, key, field, incr).Result()
}

// SetWithRetry sets a key with retry logic
func (c *Client) SetWithRetry(ctx context.Context, key string, value interface{}, expiration time.Duration, maxRetries int) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = c.Set(ctx, key, value, expiration)
		if err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return err
}

// GetOrSet gets a key, or sets it if it doesn't exist
func (c *Client) GetOrSet(ctx context.Context, key string, value interface{}, expiration time.Duration) (string, error) {
	// Try to get
	result, err := c.Get(ctx, key)
	if err == nil {
		return result, nil
	}

	// Set if not exists
	err = c.Set(ctx, key, value, expiration)
	if err != nil {
		return "", err
	}

	return c.Get(ctx, key)
}

// LockKey acquires a distributed lock
func (c *Client) LockKey(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return c.SetNX(ctx, key, "locked", expiration)
}

// UnlockKey releases a distributed lock
func (c *Client) UnlockKey(ctx context.Context, key string) error {
	return c.Del(ctx, key)
}

// FlushDB flushes the current database (use with caution!)
func (c *Client) FlushDB(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}

// FlushAll flushes all databases (use with extreme caution!)
func (c *Client) FlushAll(ctx context.Context) error {
	return c.client.FlushAll(ctx).Err()
}

// DBSize returns the number of keys in the current database
func (c *Client) DBSize(ctx context.Context) (int64, error) {
	return c.client.DBSize(ctx).Result()
}

// Info returns server information
func (c *Client) Info(ctx context.Context, section ...string) (string, error) {
	return c.client.Info(ctx, section...).Result()
}

