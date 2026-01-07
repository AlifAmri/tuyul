# Database Schema (Redis)

## Overview

TUYUL uses Redis as the primary database for all application data. Redis provides fast access, built-in data structures (hashes, sets, sorted sets), and pub/sub capabilities for real-time features.

---

## Redis Instance Configuration

```
Host: localhost (or production host)
Port: 6379
Database: 0 (default)
Max Connections: 100
Idle Timeout: 5 minutes
Connection Timeout: 5 seconds
```

---

## Data Structures

### 1. Users

#### User Object
```
Key: user:{user_id}
Type: Hash
Fields:
  id               → UUID (e.g., "550e8400-e29b-41d4-a716-446655440000")
  username         → String (unique, lowercase)
  email            → String (unique)
  password_hash    → String (bcrypt hash)
  role             → String ("admin" or "user")
  status           → String ("active", "inactive", "suspended")
  created_at       → Unix timestamp (milliseconds)
  updated_at       → Unix timestamp (milliseconds)
  last_login_at    → Unix timestamp (milliseconds)

Example:
HSET user:550e8400-e29b-41d4-a716-446655440000 
  id "550e8400-e29b-41d4-a716-446655440000"
  username "john_doe"
  email "john@example.com"
  password_hash "$2a$12$..."
  role "user"
  status "active"
  created_at "1704643200000"
  updated_at "1704643200000"
  last_login_at "1704729600000"
```

#### Username Index
```
Key: username_index:{username}
Type: String
Value: {user_id}

Example:
SET username_index:john_doe "550e8400-e29b-41d4-a716-446655440000"
```

#### Email Index
```
Key: email_index:{email}
Type: String
Value: {user_id}

Example:
SET email_index:john@example.com "550e8400-e29b-41d4-a716-446655440000"
```

#### All Users Set
```
Key: users:all
Type: Set
Members: {user_id}

Example:
SADD users:all "550e8400-e29b-41d4-a716-446655440000"
```

#### Users by Role
```
Key: users:role:{role}
Type: Set
Members: {user_id}

Examples:
SADD users:role:admin "uuid-1"
SADD users:role:user "uuid-2" "uuid-3"
```

---

### 2. Sessions

#### Session Object
```
Key: session:{user_id}
Type: String (JSON)
Value: {
  "user_id": "uuid",
  "access_token": "jwt_token",
  "refresh_token": "jwt_token",
  "expires_at": timestamp,
  "created_at": timestamp
}
TTL: 7 days (168 hours)

Example:
SETEX session:550e8400-e29b-41d4-a716-446655440000 604800
  '{"user_id":"550e8400-e29b-41d4-a716-446655440000","access_token":"eyJ..."}'
```

#### Token Blacklist
```
Key: token_blacklist:{token_hash}
Type: String
Value: "1"
TTL: Same as token expiry

Purpose: Prevent reuse of logged-out tokens

Example:
SETEX token_blacklist:abc123def456 900 "1"
```

---

### 3. API Keys

#### API Key Object
```
Key: apikey:{user_id}
Type: Hash
Fields:
  id                 → UUID
  user_id            → UUID
  encrypted_key      → String (base64 encoded)
  encrypted_secret   → String (base64 encoded)
  nonce              → String (base64 encoded, for decryption)
  label              → String (user-friendly name)
  permissions        → JSON array ["trade", "info"]
  is_active          → Boolean ("true" or "false")
  last_used_at       → Unix timestamp (milliseconds)
  created_at         → Unix timestamp (milliseconds)
  updated_at         → Unix timestamp (milliseconds)

Example:
HSET apikey:550e8400-e29b-41d4-a716-446655440000
  id "apikey-uuid"
  user_id "550e8400-e29b-41d4-a716-446655440000"
  encrypted_key "aGVsbG93b3JsZA=="
  encrypted_secret "c2VjcmV0a2V5"
  nonce "bm9uY2U="
  label "My Trading Key"
  permissions '["trade","info"]'
  is_active "true"
  last_used_at "1704729600000"
  created_at "1704643200000"
  updated_at "1704643200000"
```

#### API Key Cache (Decrypted)
```
Key: apikey_cache:{user_id}
Type: String (JSON)
Value: {
  "key": "decrypted_api_key",
  "secret": "decrypted_api_secret"
}
TTL: 1 hour (3600 seconds)

Note: This is the only place decrypted keys exist in Redis
Should be cleared on logout

Example:
SETEX apikey_cache:550e8400-e29b-41d4-a716-446655440000 3600
  '{"key":"ABCDEF123","secret":"secret123"}'
```

#### API Key by ID Index
```
Key: apikey_by_id:{apikey_id}
Type: String
Value: {user_id}

Example:
SET apikey_by_id:apikey-uuid "550e8400-e29b-41d4-a716-446655440000"
```

---

### 4. Orders

#### Order Object
```
Key: order:{order_id}
Type: Hash
Fields:
  id                  → UUID
  user_id             → UUID
  pair                → String (e.g., "btcidr")
  side                → String ("buy" or "sell")
  order_type          → String ("limit" or "market")
  price               → Float (as string)
  amount              → Float (as string)
  amount_idr          → Float (as string)
  filled              → Float (as string)
  status              → String ("pending", "open", "filled", "cancelled", "failed", "stopped")
  indodax_order_id    → String
  target_profit       → Float (percentage)
  stop_loss           → Float (percentage)
  buy_order_id        → UUID (for sell orders)
  sell_order_id       → UUID (for buy orders)
  created_at          → Unix timestamp (milliseconds)
  updated_at          → Unix timestamp (milliseconds)
  filled_at           → Unix timestamp (milliseconds, optional)
  cancelled_at        → Unix timestamp (milliseconds, optional)
  error_message       → String (optional)

Example:
HSET order:order-uuid-1
  id "order-uuid-1"
  user_id "550e8400-e29b-41d4-a716-446655440000"
  pair "btcidr"
  side "buy"
  order_type "limit"
  price "650000000"
  amount "0.00153846"
  amount_idr "1000000"
  filled "0.00153846"
  status "filled"
  indodax_order_id "123456789"
  target_profit "5.0"
  stop_loss "3.0"
  sell_order_id "order-uuid-2"
  created_at "1704729600000"
  updated_at "1704729900000"
  filled_at "1704729900000"
```

#### User Orders Index
```
Key: user_orders:{user_id}
Type: Sorted Set
Score: Created timestamp
Member: {order_id}

Purpose: Get all orders for a user, sorted by creation time

Example:
ZADD user_orders:550e8400-e29b-41d4-a716-446655440000
  1704729600000 "order-uuid-1"
  1704729900000 "order-uuid-2"

Query recent orders:
ZREVRANGE user_orders:550e8400-e29b-41d4-a716-446655440000 0 19
```

#### Orders by Status
```
Key: orders_by_status:{status}
Type: Set
Members: {order_id}

Purpose: Quick lookup of orders by status

Examples:
SADD orders_by_status:open "order-uuid-1" "order-uuid-2"
SADD orders_by_status:filled "order-uuid-3"
SADD orders_by_status:cancelled "order-uuid-4"
```

#### Orders by Pair
```
Key: pair_orders:{pair}
Type: Set
Members: {order_id}

Purpose: Find all orders for a specific trading pair (for stop-loss monitoring)

Example:
SADD pair_orders:btcidr "order-uuid-1" "order-uuid-2"
```

#### Buy-Sell Order Mapping
```
Key: buy_sell_map:{buy_order_id}
Type: String
Value: {sell_order_id}

Purpose: Link buy order to its auto-generated sell order

Example:
SET buy_sell_map:order-uuid-1 "order-uuid-2"
```

---

### 5. Market Data

#### Market Summary
```
Key: market:summary:{pair}
Type: Hash
Fields:
  pair             → String (e.g., "btcidr")
  base_currency    → String (e.g., "btc")
  quote_currency   → String (e.g., "idr")
  last_price       → Float
  high_24h         → Float
  low_24h          → Float
  volume_24h       → Float
  volume_idr       → Float
  best_bid         → Float
  best_ask         → Float
  bid_volume       → Float
  ask_volume       → Float
  change_24h       → Float (percentage)
  trade_count      → Integer
  timestamp        → Unix timestamp (milliseconds)
TTL: 10 seconds

Example:
HSET market:summary:btcidr
  pair "btcidr"
  base_currency "btc"
  quote_currency "idr"
  last_price "650000000"
  high_24h "670000000"
  low_24h "640000000"
  volume_24h "12.5"
  volume_idr "8125000000"
  best_bid "649500000"
  best_ask "650500000"
  bid_volume "0.5"
  ask_volume "0.3"
  change_24h "1.5"
  trade_count "1250"
  timestamp "1704729600000"
EXPIRE market:summary:btcidr 10
```

#### Pump Scores
```
Key: market:pump_scores
Type: Sorted Set
Score: Pump score (0-100)
Member: {pair}

Purpose: Quickly retrieve top pumping coins

Example:
ZADD market:pump_scores
  87.5 "shibidr"
  72.1 "pepeidr"
  65.3 "dogeidr"
  45.2 "btcidr"

Get top 10:
ZREVRANGE market:pump_scores 0 9 WITHSCORES
```

#### Gap Analysis
```
Key: market:gaps
Type: Sorted Set
Score: Gap percentage
Member: {pair}

Purpose: Find pairs with widest bid-ask spread

Example:
ZADD market:gaps
  0.85 "ethidr"
  0.72 "bnbidr"
  0.65 "adaidr"
  0.15 "btcidr"

Get top 10:
ZREVRANGE market:gaps 0 9 WITHSCORES
```

#### Market Cache (All Pairs)
```
Key: market:cache:all
Type: String (JSON array)
Value: [...all market summaries...]
TTL: 2 seconds

Purpose: Fast response for /api/v1/market/summary endpoint

Example:
SETEX market:cache:all 2 '[{"pair":"btcidr",...},{"pair":"ethidr",...}]'
```

#### Active Pairs Set
```
Key: market:active_pairs
Type: Set
Members: {pair}

Purpose: Track which pairs are currently active

Example:
SADD market:active_pairs "btcidr" "ethidr" "bnbidr"
```

---

### 6. Balances

#### User Balance Cache
```
Key: balance:{user_id}:{currency}
Type: String (JSON)
Value: {
  "currency": "idr",
  "available": 5000000.0,
  "frozen": 1000000.0,
  "total": 6000000.0,
  "timestamp": 1704729600000
}
TTL: 5 seconds

Purpose: Cache balance to reduce Indodax API calls

Example:
SETEX balance:550e8400-e29b-41d4-a716-446655440000:idr 5
  '{"currency":"idr","available":5000000.0,"frozen":1000000.0,"total":6000000.0}'
```

#### All Balances Cache
```
Key: balance:{user_id}:all
Type: String (JSON)
Value: {
  "idr": {...},
  "btc": {...},
  "eth": {...}
}
TTL: 5 seconds

Example:
SETEX balance:550e8400-e29b-41d4-a716-446655440000:all 5
  '{"idr":{...},"btc":{...}}'
```

---

### 7. WebSocket Connections

#### Active Connections
```
Key: ws:connections:{user_id}
Type: Set
Members: {connection_id}

Purpose: Track active WebSocket connections per user

Example:
SADD ws:connections:550e8400-e29b-41d4-a716-446655440000
  "conn-uuid-1" "conn-uuid-2"
```

#### Connection Info
```
Key: ws:connection:{connection_id}
Type: Hash
Fields:
  user_id      → UUID
  connected_at → Unix timestamp
  last_ping    → Unix timestamp
  subscriptions → JSON array (subscribed channels)

Example:
HSET ws:connection:conn-uuid-1
  user_id "550e8400-e29b-41d4-a716-446655440000"
  connected_at "1704729600000"
  last_ping "1704729900000"
  subscriptions '["market_summary","order_updates"]'
```

---

### 8. Pub/Sub Channels

#### Market Updates
```
Channel: market:updates
Message Format: {
  "type": "market_update",
  "pair": "btcidr",
  "data": {...}
}

Purpose: Broadcast market data to all connected clients
```

#### Order Updates
```
Channel: order:updates:{user_id}
Message Format: {
  "type": "order_update",
  "order_id": "uuid",
  "data": {...}
}

Purpose: Send order updates to specific user
```

#### System Notifications
```
Channel: system:notifications
Message Format: {
  "type": "system_notification",
  "level": "info|warning|error",
  "message": "..."
}

Purpose: Broadcast system-wide notifications
```

---

### 9. Rate Limiting

#### User Rate Limit
```
Key: rate_limit:user:{user_id}:{endpoint}
Type: String
Value: {count}
TTL: Varies by endpoint (e.g., 60 seconds)

Purpose: Track API requests per user per endpoint

Example:
INCR rate_limit:user:550e8400-e29b-41d4-a716-446655440000:trade
EXPIRE rate_limit:user:550e8400-e29b-41d4-a716-446655440000:trade 60

Check:
GET rate_limit:user:550e8400-e29b-41d4-a716-446655440000:trade
```

#### IP Rate Limit
```
Key: rate_limit:ip:{ip_address}:{endpoint}
Type: String
Value: {count}
TTL: Varies by endpoint

Purpose: Prevent abuse from single IP

Example:
INCR rate_limit:ip:192.168.1.1:login
EXPIRE rate_limit:ip:192.168.1.1:login 300
```

---

### 10. System Configuration

#### Pair Configuration
```
Key: config:pair:{pair}
Type: Hash
Fields:
  pair              → String
  base_currency     → String
  quote_currency    → String
  price_increment   → Float
  min_order_value   → Float (in IDR)
  trading_enabled   → Boolean
  updated_at        → Unix timestamp

Example:
HSET config:pair:btcidr
  pair "btcidr"
  base_currency "btc"
  quote_currency "idr"
  price_increment "1000"
  min_order_value "10000"
  trading_enabled "true"
  updated_at "1704643200000"
```

#### System Settings
```
Key: config:system
Type: Hash
Fields:
  maintenance_mode   → Boolean
  maintenance_message → String
  min_volume_filter  → Float (for gap analysis)
  max_open_orders    → Integer (per user)

Example:
HSET config:system
  maintenance_mode "false"
  min_volume_filter "10000000"
  max_open_orders "50"
```

---

## Indexing Strategy

### 1. Primary Keys
- Users: `user:{user_id}`
- Orders: `order:{order_id}`
- API Keys: `apikey:{user_id}`

### 2. Secondary Indices
- Username → User ID: `username_index:{username}`
- Email → User ID: `email_index:{email}`
- Order by User: `user_orders:{user_id}` (sorted set by timestamp)
- Order by Status: `orders_by_status:{status}` (set)
- Order by Pair: `pair_orders:{pair}` (set)

### 3. Lookups
- Find user by username: `GET username_index:{username}` → `HGETALL user:{user_id}`
- Find user's orders: `ZREVRANGE user_orders:{user_id} 0 -1` → `HGETALL order:{order_id}`
- Find open orders: `SMEMBERS orders_by_status:open` → `HGETALL order:{order_id}`

---

## Data Lifecycle

### Session Data
- **Created**: On login
- **TTL**: 7 days
- **Refreshed**: On token refresh
- **Deleted**: On logout or expiry

### Market Data
- **Updated**: Every WebSocket message from Indodax
- **TTL**: 10 seconds (stale if not updated)
- **Cleanup**: Automatic via Redis TTL

### Order Data
- **Created**: On order placement
- **Updated**: On order fill/cancel
- **Deleted**: Never (kept for history)
- **Archived**: Move to cold storage after 90 days (future)

### Balance Cache
- **Updated**: After order placement, after fill
- **TTL**: 5 seconds
- **Invalidated**: On order events

---

## Redis Memory Management

### Estimated Memory Usage (per 1000 users)

```
Users:              ~500 KB (500 bytes per user)
Sessions:           ~300 KB (300 bytes per session)
API Keys:           ~400 KB (400 bytes per key)
Orders (active):    ~2 MB (20 orders per user, 100 bytes each)
Market Data:        ~200 KB (200 pairs, 1 KB each)
Balances:           ~100 KB (cached, short TTL)
WebSocket:          ~150 KB (connection tracking)

Total:              ~3.65 MB per 1000 users
```

### Memory Optimization
- Use hashes for objects (more memory efficient than JSON strings)
- Set appropriate TTLs on cached data
- Use sorted sets for time-based queries
- Compress large JSON values (optional)

### Eviction Policy
```
maxmemory-policy: allkeys-lru
```
Evict least recently used keys when memory limit reached.

---

## Backup Strategy

### Daily Backups
```bash
# RDB snapshot (point-in-time backup)
redis-cli SAVE
# Or use BGSAVE for background save

# Copy dump.rdb to backup location
cp /var/lib/redis/dump.rdb /backups/redis-$(date +%Y%m%d).rdb
```

### AOF (Append-Only File)
Enable for durability:
```
appendonly yes
appendfsync everysec
```

---

## Redis Commands Reference

### Common Operations

#### Create User
```redis
HSET user:{uuid} id {uuid} username {username} email {email} ...
SET username_index:{username} {uuid}
SET email_index:{email} {uuid}
SADD users:all {uuid}
SADD users:role:user {uuid}
```

#### Get User by Username
```redis
GET username_index:{username}  # Returns user_id
HGETALL user:{user_id}
```

#### Create Order
```redis
HSET order:{uuid} id {uuid} user_id {user_id} pair {pair} ...
ZADD user_orders:{user_id} {timestamp} {uuid}
SADD orders_by_status:open {uuid}
SADD pair_orders:{pair} {uuid}
```

#### Get User's Recent Orders
```redis
ZREVRANGE user_orders:{user_id} 0 19  # Get order IDs
HGETALL order:{order_id}  # For each order ID
```

#### Update Order Status
```redis
HSET order:{uuid} status filled filled_at {timestamp}
SREM orders_by_status:open {uuid}
SADD orders_by_status:filled {uuid}
```

#### Store Market Data
```redis
HSET market:summary:{pair} last_price {price} volume_24h {volume} ...
EXPIRE market:summary:{pair} 10
ZADD market:pump_scores {score} {pair}
```

#### Get Top Pumping Coins
```redis
ZREVRANGE market:pump_scores 0 9 WITHSCORES
```

---

## Performance Considerations

### Connection Pooling
- Maintain pool of 50-100 Redis connections
- Reuse connections across requests
- Set idle timeout to 5 minutes

### Pipeline Operations
- Batch multiple commands in single round-trip
- Especially useful for bulk updates

Example:
```go
pipe := redis.Pipeline()
pipe.HSet(ctx, "order:uuid", "status", "filled")
pipe.SRem(ctx, "orders_by_status:open", "uuid")
pipe.SAdd(ctx, "orders_by_status:filled", "uuid")
pipe.Exec(ctx)
```

### Lua Scripts
- Atomic operations on multiple keys
- Reduce network round-trips

Example:
```lua
-- Atomic order status update
local order_id = ARGV[1]
local old_status = ARGV[2]
local new_status = ARGV[3]

redis.call('HSET', 'order:' .. order_id, 'status', new_status)
redis.call('SREM', 'orders_by_status:' .. old_status, order_id)
redis.call('SADD', 'orders_by_status:' .. new_status, order_id)
return 1
```

---

## Monitoring

### Key Metrics to Monitor
- Memory usage: `INFO memory`
- Hit/miss ratio: `INFO stats`
- Connected clients: `INFO clients`
- Commands/sec: `INFO stats`
- Slow queries: `SLOWLOG GET 10`
- Key distribution: `DBSIZE`

### Alerts
- Memory usage > 80%
- Hit rate < 90%
- Connected clients > 1000
- Slow queries > 100ms

---

## Migration Strategy

### From Development to Production
1. Export RDB snapshot from dev
2. Import to production Redis
3. Verify data integrity
4. Switch application to production Redis
5. Monitor for issues

### Schema Changes
1. Update application code (backward compatible)
2. Deploy new version
3. Run migration script
4. Verify data
5. Remove old schema (if needed)

---

## Future Considerations

- **Sharding**: If data grows beyond single Redis instance
- **Redis Cluster**: For horizontal scaling
- **Sentinel**: For high availability
- **Redis Enterprise**: For production-grade features
- **TimeSeries**: For historical market data (Redis TimeSeries module)

