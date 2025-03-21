package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var DB *pgxpool.Pool

func InitDBPool(dbConnString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dbConnString)
	if err != nil {
		return nil, err
	}

	// Configure the connection pool
	config.MaxConns = 100                      // Maximum number of connections
	config.MinConns = 10                       // Minimum idle connections
	config.MaxConnLifetime = 1 * time.Hour     // Maximum connection lifetime
	config.MaxConnIdleTime = 30 * time.Minute  // Maximum idle time
	config.HealthCheckPeriod = 1 * time.Minute // Health check period

	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	return pool, nil
}

func SetupDatabase(dbPool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create users table
	_, err := dbPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(50) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			email VARCHAR(255) UNIQUE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			last_login TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	// Create auth_tokens table
	_, err = dbPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS auth_tokens (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			token VARCHAR(255) UNIQUE NOT NULL,
			ip_address VARCHAR(50),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMP NOT NULL,
			last_used_at TIMESTAMP,
			is_revoked BOOLEAN NOT NULL DEFAULT FALSE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create auth_tokens table: %w", err)
	}

	// Create indexes for faster lookups
	_, err = dbPool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_auth_tokens_token ON auth_tokens(token);
		CREATE INDEX IF NOT EXISTS idx_auth_tokens_user_id ON auth_tokens(user_id);
	`)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	log.Println("Database tables initialized successfully")
	return nil
}
