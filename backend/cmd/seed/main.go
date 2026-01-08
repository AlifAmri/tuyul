package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"tuyul/backend/internal/config"
	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/pkg/crypto"
	"tuyul/backend/pkg/redis"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	_ = godotenv.Load()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize Redis
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

	// Initialize Redis key prefix
	redis.InitKeys(cfg.Redis.Prefix)

	userRepo := repository.NewUserRepository(redisClient)

	// Create admin user
	username := "alifamri@evn.co.id"
	password := "beungeut"
	email := "alifamri@evn.co.id"

	// Hash password
	passwordHash, err := crypto.HashPassword(password)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	user := &model.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         model.RoleAdmin,
		Status:       model.StatusActive,
		HasAPIKey:    false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	ctx := context.Background()

	// Check if already exists
	existing, _ := userRepo.GetByUsername(ctx, username)
	if existing != nil {
		fmt.Printf("User %s already exists. Updating password and role to admin...\n", username)
		existing.PasswordHash = passwordHash
		existing.Role = model.RoleAdmin
		existing.Status = model.StatusActive
		if err := userRepo.Update(ctx, existing); err != nil {
			log.Fatalf("Failed to update user: %v", err)
		}
		fmt.Println("✓ User updated successfully")
		return
	}

	if err := userRepo.Create(ctx, user); err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	fmt.Printf("✓ Admin user created successfully:\n")
	fmt.Printf("  Username: %s\n", username)
	fmt.Printf("  Password: %s\n", password)
	fmt.Printf("  Role:     %s\n", model.RoleAdmin)
}
