package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration loaded from environment variables and .env file.
type Config struct {
	DataDir string
}

// Load loads configuration from environment variables, with optional .env file as fallback.
// Environment variables take precedence over .env values.
func Load() (*Config, error) {
	// Try to load .env file, but don't error if it doesn't exist
	_ = godotenv.Load()

	cfg := &Config{
		DataDir: os.Getenv("TEMPORALDB_DATA_DIR"),
	}

	return cfg, nil
}
