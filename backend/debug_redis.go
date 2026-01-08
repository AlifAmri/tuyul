package main

import (
	"context"
	"fmt"
	"log"

	"tuyul/backend/internal/config"
	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/redis"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg, _ := config.Load()

	redisClient, err := redis.New(redis.Config{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatalf("Redis error: %v", err)
	}
	defer redisClient.Close()

	redis.InitKeys(cfg.Redis.Prefix)

	ctx := context.Background()
	username := "alifamri@evn.co.id"
	usernameKey := redis.UserByUsernameKey(username)

	fmt.Printf("Searching for username key: %s\n", usernameKey)

	userID, err := redisClient.Get(ctx, usernameKey)
	if err != nil {
		log.Fatalf("Get userID failed: %v", err)
	}
	fmt.Printf("Found userID: %s\n", userID)

	userKey := redis.UserKey(userID)
	var user model.User
	err = redisClient.GetJSON(ctx, userKey, &user)
	if err != nil {
		log.Fatalf("Get user failed: %v", err)
	}

	fmt.Printf("User found: %+v\n", user)
	fmt.Printf("PasswordHash stored: %s\n", user.PasswordHash)
}
