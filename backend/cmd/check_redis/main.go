package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"tuyul/backend/internal/config"
	"tuyul/backend/pkg/redis"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	redisClient, err := redis.New(redis.Config{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	redis.InitKeys(cfg.Redis.Prefix)
	ctx := context.Background()

	// Check for coin keys
	pattern := redis.CoinKey("*")
	keys, err := redisClient.Keys(ctx, pattern)
	if err != nil {
		log.Fatalf("Failed to scan keys: %v", err)
	}

	fmt.Printf("Found %d coins in Redis with pattern %s\n", len(keys), pattern)
	if len(keys) > 0 {
		fmt.Println("First 5 keys:")
		for i := 0; i < len(keys) && i < 5; i++ {
			fmt.Println("- ", keys[i])
		}
	}

	// Check for metadata keys
	pairsKey := redis.CachePairsKey()
	exists, _ := redisClient.Exists(ctx, pairsKey)
	fmt.Printf("Metadata Pairs Key (%s) exists: %v\n", pairsKey, exists)

	// DEBUG: Check specific coin cstidr
	cstKey := redis.CoinKey("cstidr")
	cstExists, _ := redisClient.Exists(ctx, cstKey)
	fmt.Printf("Coin Key (%s) exists: %v\n", cstKey, cstExists)
	if cstExists {
		data, _ := redisClient.HGetAll(ctx, cstKey)
		fmt.Printf("Data for cstidr: %+v\n", data)
	}
}
