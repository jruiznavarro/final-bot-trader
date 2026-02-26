package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the application configuration
type Config struct {
	// Server configuration
	ServerPort string

	// Database configuration
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Bitunix exchange configuration
	BitunixAPIKey    string
	BitunixSecretKey string
	BitunixBaseURL   string

	// Trading configuration
	DefaultLeverage   int
	DefaultMarginType string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		// Server defaults
		ServerPort: getEnv("SERVER_PORT", "8080"),

		// Database defaults
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "trader_user"),
		DBPassword: getEnv("DB_PASSWORD", "trader_password"),
		DBName:     getEnv("DB_NAME", "final_bot_trader_db"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		// Bitunix configuration
		BitunixAPIKey:    getEnv("BITUNIX_API_KEY", ""),
		BitunixSecretKey: getEnv("BITUNIX_SECRET_KEY", ""),
		BitunixBaseURL:   getEnv("BITUNIX_BASE_URL", "https://fapi.bitunix.com"),

		// Trading defaults
		DefaultLeverage:   getEnvAsInt("DEFAULT_LEVERAGE", 10),
		DefaultMarginType: getEnv("DEFAULT_MARGIN_TYPE", "ISOLATED"),
	}

	return cfg, nil
}

// Validate checks if required configuration is present
func (c *Config) Validate() error {
	if c.BitunixAPIKey == "" {
		return fmt.Errorf("BITUNIX_API_KEY is required")
	}
	if c.BitunixSecretKey == "" {
		return fmt.Errorf("BITUNIX_SECRET_KEY is required")
	}
	return nil
}

// DatabaseDSN returns the database connection string
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

// getEnv returns the value of an environment variable or a default
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvAsInt returns the value of an environment variable as int or a default
func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvAsBool returns the value of an environment variable as bool or a default
func getEnvAsBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
