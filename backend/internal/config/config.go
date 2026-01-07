package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	Server     ServerConfig
	Redis      RedisConfig
	JWT        JWTConfig
	Encryption EncryptionConfig
	Indodax    IndodaxConfig
	CORS       CORSConfig
	RateLimit  RateLimitConfig
	Log        LogConfig
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Host string
	Port string
	Env  string
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// JWTConfig holds JWT token configuration
type JWTConfig struct {
	Secret               string
	AccessTokenExpire    time.Duration
	RefreshTokenExpire   time.Duration
}

// EncryptionConfig holds encryption configuration for API keys
type EncryptionConfig struct {
	Key string
}

// IndodaxConfig holds Indodax API configuration
type IndodaxConfig struct {
	APIURL        string
	WSURL         string
	PrivateWSURL  string
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins []string
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute     int
	AuthRequestsPerMinute int
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnv("SERVER_PORT", "8080"),
			Env:  getEnv("SERVER_ENV", "development"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:               getEnv("JWT_SECRET", ""),
			AccessTokenExpire:    time.Duration(getEnvAsInt("JWT_ACCESS_TOKEN_EXPIRE_MINUTES", 15)) * time.Minute,
			RefreshTokenExpire:   time.Duration(getEnvAsInt("JWT_REFRESH_TOKEN_EXPIRE_DAYS", 7)) * 24 * time.Hour,
		},
		Encryption: EncryptionConfig{
			Key: getEnv("ENCRYPTION_KEY", ""),
		},
		Indodax: IndodaxConfig{
			APIURL:       getEnv("INDODAX_API_URL", "https://indodax.com"),
			WSURL:        getEnv("INDODAX_WS_URL", "wss://ws3.indodax.com/ws/"),
			PrivateWSURL: getEnv("INDODAX_PRIVATE_WS_URL", "wss://pws.indodax.com/ws/"),
		},
		CORS: CORSConfig{
			AllowedOrigins: getEnvAsSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173"}, ","),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute:     getEnvAsInt("RATE_LIMIT_REQUESTS_PER_MINUTE", 60),
			AuthRequestsPerMinute: getEnvAsInt("RATE_LIMIT_AUTH_REQUESTS_PER_MINUTE", 5),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
	}

	// Validate required fields
	if cfg.JWT.Secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	if cfg.Encryption.Key == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required")
	}

	if len(cfg.Encryption.Key) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes")
	}

	return cfg, nil
}

// Address returns the full server address
func (c *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

// RedisAddress returns the full Redis address
func (c *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

// IsDevelopment returns true if running in development mode
func (c *ServerConfig) IsDevelopment() bool {
	return c.Env == "development"
}

// IsProduction returns true if running in production mode
func (c *ServerConfig) IsProduction() bool {
	return c.Env == "production"
}

// Helper functions

func getEnv(key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsSlice(key string, defaultValue []string, separator string) []string {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	return strings.Split(valueStr, separator)
}

