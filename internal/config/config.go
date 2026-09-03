package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration loaded from environment variables and .env file.
type Config struct {
	Addr           string
	DataDir        string
	BackupDir      string
	BackupInterval string
	Retention      string
	QdrantURL      string
	QdrantAPIKey   string
	TEIURL         string
	TEIRerankURL   string
}

// Load loads configuration from environment variables, with optional .env file as fallback.
// Environment variables take precedence over .env values.
func Load() (*Config, error) {
	// Try to load .env file, but don't error if it doesn't exist
	_ = godotenv.Load()

	cfg := &Config{
		Addr:           os.Getenv("TEMPORALDB_ADDR"),
		DataDir:        os.Getenv("TEMPORALDB_DATA_DIR"),
		BackupDir:      os.Getenv("TEMPORALDB_BACKUP_DIR"),
		BackupInterval: os.Getenv("TEMPORALDB_BACKUP_INTERVAL"),
		Retention:      os.Getenv("TEMPORALDB_RETENTION"),
		QdrantURL:      os.Getenv("QDRANT_URL"),
		QdrantAPIKey:   os.Getenv("QDRANT_API_KEY"),
		TEIURL:         os.Getenv("TEI_URL"),
		TEIRerankURL:   os.Getenv("TEI_RERANK_URL"),
	}

	return cfg, nil
}
