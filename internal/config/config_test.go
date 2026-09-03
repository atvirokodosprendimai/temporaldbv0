package config

import (
	"os"
	"testing"
)

func TestLoadPrefersEnvOverDotEnv(t *testing.T) {
	// Set an env var
	t.Setenv("TEMPORALDB_DATA_DIR", "/tmp/from-env")

	// Load config
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Env should win over any .env value
	if cfg.DataDir != "/tmp/from-env" {
		t.Errorf("expected DataDir=/tmp/from-env, got %q", cfg.DataDir)
	}
}

func TestLoadEnvAbsent(t *testing.T) {
	// Clear the env vars to test that Load works even without .env
	os.Clearenv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should not error when .env is absent, got: %v", err)
	}

	// Should return a Config with defaults or from env
	if cfg == nil {
		t.Fatal("expected Config, got nil")
	}
}
