package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tuyul/backend/internal/config"
	"tuyul/backend/internal/handler"
	"tuyul/backend/internal/middleware"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/service"
	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/jwt"
	"tuyul/backend/pkg/logger"
	"tuyul/backend/pkg/redis"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file (ignore error in production)
	_ = godotenv.Load()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger.Init(cfg.Log.Level, cfg.Log.Format)
	log := logger.GetLogger()

	log.Info("Starting TUYUL Backend...")
	log.Infof("Environment: %s", cfg.Server.Env)

	// Initialize Redis
	log.Info("Connecting to Redis...")
	redisClient, err := redis.New(redis.Config{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatal("Failed to connect to Redis", err)
	}
	defer redisClient.Close()
	log.Info("✓ Redis connected")

	// Set Gin mode
	if cfg.Server.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.New()

	// Apply middleware
	router.Use(middleware.Recovery(log))          // Panic recovery
	router.Use(middleware.RequestID())            // Request ID
	router.Use(middleware.Logger(log))            // Request logging
	router.Use(middleware.CORS(cfg.CORS.AllowedOrigins)) // CORS
	router.Use(middleware.RateLimit(redisClient, cfg.RateLimit.RequestsPerMinute)) // Rate limiting

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		// Test Redis connection
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := redisClient.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "Redis connection failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"redis":  "connected",
		})
	})

	// Initialize JWT manager
	jwtManager := jwt.NewJWTManager(
		cfg.JWT.Secret,
		cfg.JWT.AccessTokenExpire,
		cfg.JWT.RefreshTokenExpire,
	)

	// Initialize Indodax client
	indodaxClient := indodax.NewClient(cfg.Indodax.APIURL)

	// Initialize repositories
	userRepo := repository.NewUserRepository(redisClient)
	apiKeyRepo := repository.NewAPIKeyRepository(redisClient)

	// Initialize services
	authService := service.NewAuthService(userRepo, jwtManager)
	userService := service.NewUserService(userRepo)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, userRepo, indodaxClient, cfg.Encryption.Key)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	userHandler := handler.NewUserHandler(userService)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyService)

	// API v1 group
	v1 := router.Group("/api/v1")
	{
		// Public routes
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "pong",
				"time":    time.Now().Unix(),
			})
		})

		// Auth routes
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", middleware.AuthRateLimit(redisClient, cfg.RateLimit.AuthRequestsPerMinute), authHandler.Login)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/logout", middleware.AuthMiddleware(authService), authHandler.Logout)
			auth.GET("/me", middleware.AuthMiddleware(authService), authHandler.GetMe)
		}

		// User routes
		users := v1.Group("/users")
		users.Use(middleware.AuthMiddleware(authService))
		{
			// Current user routes
			users.GET("/profile", userHandler.GetProfile)
			users.PUT("/profile", userHandler.UpdateProfile)
			users.POST("/password", userHandler.ChangePassword)

			// Admin only routes
			admin := users.Group("")
			admin.Use(middleware.RequireAdmin())
			{
				admin.GET("", userHandler.ListUsers)
				admin.POST("", userHandler.CreateUser)
				admin.GET("/:id", userHandler.GetUser)
				admin.PUT("/:id", userHandler.UpdateUser)
				admin.DELETE("/:id", userHandler.DeleteUser)
				admin.POST("/:id/reset-password", userHandler.ResetPassword)
			}
		}

		// API Key routes
		apiKeys := v1.Group("/api-keys")
		apiKeys.Use(middleware.AuthMiddleware(authService))
		{
			apiKeys.POST("", apiKeyHandler.Create)
			apiKeys.GET("", apiKeyHandler.Get)
			apiKeys.DELETE("", apiKeyHandler.Delete)
			apiKeys.POST("/validate", apiKeyHandler.Validate)
			apiKeys.GET("/account-info", apiKeyHandler.GetAccountInfo)
		}
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Infof("Server starting on %s", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server", err)
		}
	}()

	log.Info("✓ Server started successfully")

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Graceful shutdown with 5 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown", err)
	}

	log.Info("Server exited")
}

