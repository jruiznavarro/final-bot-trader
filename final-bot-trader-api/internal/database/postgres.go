package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// DB represents the database connection
type DB struct {
	*sql.DB
}

// Config holds database configuration
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// ConfigFromEnv loads database config from environment variables
func ConfigFromEnv() Config {
	return Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "trader_user"),
		Password: getEnv("DB_PASSWORD", "trader_password"),
		DBName:   getEnv("DB_NAME", "final_bot_trader_db"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Connect establishes a database connection
func Connect(config Config) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DBName, config.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Printf("Connected to database: %s@%s:%s/%s", config.User, config.Host, config.Port, config.DBName)

	return &DB{db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// RunMigrations runs database migrations
func (db *DB) RunMigrations() error {
	migrations := []string{
		// Trades table
		`CREATE TABLE IF NOT EXISTS trades (
			id VARCHAR(255) PRIMARY KEY,
			symbol VARCHAR(50) NOT NULL,
			side VARCHAR(10) NOT NULL,
			entry_price DECIMAL(20, 8) NOT NULL,
			exit_price DECIMAL(20, 8),
			quantity DECIMAL(20, 8) NOT NULL,
			stop_loss DECIMAL(20, 8),
			take_profit DECIMAL(20, 8),
			pnl DECIMAL(20, 8),
			status VARCHAR(20) NOT NULL DEFAULT 'OPEN',
			reason TEXT,
			exit_reason TEXT,
			order_id BIGINT,
			position_id VARCHAR(255),
			entry_time TIMESTAMP WITH TIME ZONE NOT NULL,
			exit_time TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		// Index on symbol for faster lookups
		`CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades(symbol)`,
		// Index on status for filtering open/closed trades
		`CREATE INDEX IF NOT EXISTS idx_trades_status ON trades(status)`,
		// Index on entry_time for date range queries
		`CREATE INDEX IF NOT EXISTS idx_trades_entry_time ON trades(entry_time)`,

		// Daily statistics table
		`CREATE TABLE IF NOT EXISTS daily_stats (
			date DATE PRIMARY KEY,
			total_trades INT DEFAULT 0,
			wins INT DEFAULT 0,
			losses INT DEFAULT 0,
			total_pnl DECIMAL(20, 8) DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,

		// Bot sessions table
		`CREATE TABLE IF NOT EXISTS bot_sessions (
			id SERIAL PRIMARY KEY,
			start_time TIMESTAMP WITH TIME ZONE NOT NULL,
			end_time TIMESTAMP WITH TIME ZONE,
			mode VARCHAR(20) NOT NULL,
			position_size DECIMAL(20, 8),
			leverage INT,
			total_trades INT DEFAULT 0,
			total_pnl DECIMAL(20, 8) DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	log.Println("Database migrations completed successfully")
	return nil
}
