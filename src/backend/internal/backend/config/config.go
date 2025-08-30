package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config represents the application configuration
type Config struct {
	Database DatabaseConfig
	Storage  StorageConfig
	Server   ServerConfig
}

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	Path string
	Mode int
}

// StorageConfig represents storage configuration
type StorageConfig struct {
	Path string
}

// ServerConfig represents server configuration
type ServerConfig struct {
	RestPort int
	GrpcPort int
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{}

	// Database configuration
	config.Database.Path = getEnv("DB_PATH", "./scriptorium.db")

	dbModeStr := getEnv("DB_MODE", "0600")
	dbMode, err := strconv.ParseInt(dbModeStr, 8, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MODE: %s", dbModeStr)
	}
	config.Database.Mode = int(dbMode)

	// Storage configuration
	config.Storage.Path = getEnv("STORAGE_PATH", "./storage")

	// Server configuration
	restPortStr := getEnv("REST_PORT", "8080")
	restPort, err := strconv.Atoi(restPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid REST_PORT: %s", restPortStr)
	}
	config.Server.RestPort = restPort

	grpcPortStr := getEnv("GRPC_PORT", "5001")
	grpcPort, err := strconv.Atoi(grpcPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid GRPC_PORT: %s", grpcPortStr)
	}
	config.Server.GrpcPort = grpcPort

	return config, nil
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
