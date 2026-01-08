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
	"tuyul/backend/internal/model"
	"tuyul/backend/internal/repository"
	"tuyul/backend/internal/service"
	"tuyul/backend/internal/service/market"
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

	// Initialize Redis key prefix
	redis.InitKeys(cfg.Redis.Prefix)

	// Set Gin mode
	if cfg.Server.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.New()

	// Apply middleware
	router.Use(middleware.Recovery(log))                 // Panic recovery
	router.Use(middleware.RequestID())                   // Request ID
	router.Use(middleware.Logger(log))                   // Request logging
	router.Use(middleware.CORS(cfg.CORS.AllowedOrigins)) // CORS
	// router.Use(middleware.RateLimit(redisClient, cfg.RateLimit.RequestsPerMinute)) // Rate limiting

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
	tradeRepo := repository.NewTradeRepository(redisClient)
	botRepo := repository.NewBotRepository(redisClient)
	orderRepo := repository.NewOrderRepository(redisClient)
	posRepo := repository.NewPositionRepository(redisClient)
	balanceRepo := repository.NewBalanceRepository(redisClient)

	// Initialize services
	notificationService := service.NewNotificationService(redisClient)
	authService := service.NewAuthService(userRepo, jwtManager)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, userRepo, indodaxClient, notificationService, cfg.Encryption.Key)
	userService := service.NewUserService(userRepo, botRepo, tradeRepo, apiKeyService)

	// Initialize Market Analysis services
	publicWSClient := indodax.NewWSClient(cfg.Indodax.WSURL, cfg.Indodax.WSToken)
	marketDataService := market.NewMarketDataService(redisClient, publicWSClient, indodaxClient)
	subManager := market.NewSubscriptionManager(publicWSClient)
	timeframeManager := market.NewTimeframeManager(marketDataService, redisClient)

	// Start Market Analysis
	marketDataService.Start()
	timeframeManager.Start()

	// Initialize Order Monitor
	orderMonitor := service.NewOrderMonitor(tradeRepo, orderRepo, apiKeyService, notificationService, indodaxClient)

	// Initialize Copilot service
	copilotService := service.NewCopilotService(tradeRepo, orderRepo, balanceRepo, apiKeyService, marketDataService, orderMonitor, indodaxClient)

	// Initialize Market Maker service
	mmService := service.NewMarketMakerService(botRepo, orderRepo, apiKeyService, marketDataService, subManager, orderMonitor, notificationService, indodaxClient)

	// Initialize Pump Hunter service
	phService := service.NewPumpHunterService(botRepo, posRepo, orderRepo, apiKeyService, marketDataService, orderMonitor, notificationService, indodaxClient)

	botHandler := handler.NewBotHandler(botRepo, mmService, phService)

	// Register Pump Signal Notifications
	marketDataService.OnUpdate(func(coin *model.Coin) {
		if coin.PumpScore >= 75 {
			notificationService.NotifyPumpSignal(context.Background(), coin)
		}
	})

	// Initialize Stop-Loss Monitor
	stopLossMonitor := service.NewStopLossMonitor(tradeRepo, apiKeyService, indodaxClient, marketDataService, notificationService, balanceRepo)

	// Initialize WebSocket Hub
	wsHub := service.NewWSHub(redisClient.GetClient())
	go wsHub.Run()
	go wsHub.StartPubSubListener(context.Background())

	// Set up callbacks for auto-sell
	orderMonitor.SetBuyFilledCallback(func(trade *model.Trade, filledAmount float64) {
		ctx := context.Background()
		if err := copilotService.PlaceAutoSell(ctx, trade, filledAmount); err != nil {
			log.Errorf("Auto-sell failed for TradeID=%d: %v", trade.ID, err)
		} else {
			// Add to stop-loss monitoring
			stopLossMonitor.AddTrade(trade)
		}
	})

	orderMonitor.SetSellFilledCallback(func(trade *model.Trade, filledAmount float64, avgPrice float64) {
		// Remove from stop-loss monitoring when sell completes
		stopLossMonitor.RemoveTrade(trade.ID)
		log.Infof("Trade completed: TradeID=%d, Profit=%.2f IDR", trade.ID, trade.ProfitIDR)
	})

	// Start monitors
	stopLossMonitor.Start()

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	userHandler := handler.NewUserHandler(userService)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyService)
	marketHandler := handler.NewMarketHandler(marketDataService)
	copilotHandler := handler.NewCopilotHandler(copilotService)

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

		v1.GET("/health", func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := redisClient.Ping(ctx)
			status := http.StatusOK
			redisStatus := "connected"
			if err != nil {
				status = http.StatusServiceUnavailable
				redisStatus = "disconnected"
			}

			c.JSON(status, gin.H{
				"status": "ready",
				"redis":  redisStatus,
				"time":   time.Now().Unix(),
			})
		})

		// Auth routes
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
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
			users.GET("/stats", userHandler.GetStats)
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

		// Market routes
		marketRoutes := v1.Group("/market")
		marketRoutes.Use(middleware.AuthMiddleware(authService))
		{
			marketRoutes.GET("/summary", marketHandler.GetSummary)
			marketRoutes.GET("/pump-scores", marketHandler.GetPumpScores)
			marketRoutes.GET("/gaps", marketHandler.GetGaps)
			marketRoutes.GET("/:pair", marketHandler.GetPairDetail)
			marketRoutes.POST("/sync", middleware.AuthMiddleware(authService), marketHandler.SyncMetadata)
		}

		// Copilot routes
		copilot := v1.Group("/copilot")
		copilot.Use(middleware.AuthMiddleware(authService))
		{
			copilot.POST("/trade", copilotHandler.PlaceTrade)
			copilot.GET("/trades", copilotHandler.GetTrades)
			copilot.GET("/trades/:id", copilotHandler.GetTrade)
			copilot.DELETE("/trades/:id", copilotHandler.CancelTrade)
			copilot.POST("/trades/:id/sell", copilotHandler.ManualSell)
		}

		// Bot routes
		bots := v1.Group("/bots")
		bots.Use(middleware.AuthMiddleware(authService))
		{
			bots.POST("", botHandler.CreateBot)
			bots.GET("", botHandler.ListBots)
			bots.GET("/:id", botHandler.GetBot)
			bots.PUT("/:id", botHandler.UpdateBot)
			bots.GET("/:id/summary", botHandler.GetBotSummary)
			bots.DELETE("/:id", botHandler.DeleteBot)
			bots.POST("/:id/start", botHandler.StartBot)
			bots.POST("/:id/stop", botHandler.StopBot)
			bots.GET("/:id/positions", botHandler.ListPositions)
		}

		// WebSocket route
		v1.GET("/ws", middleware.AuthMiddleware(authService), wsHub.ServeWS)
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
