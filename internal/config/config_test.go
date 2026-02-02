// internal/config/config_test.go
package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if !cfg.Models.Claude.Enabled {
		t.Error("Claude should be enabled by default")
	}
	if cfg.Models.Claude.CLIPath != "claude" {
		t.Errorf("Claude CLI path should be 'claude', got %s", cfg.Models.Claude.CLIPath)
	}
	if cfg.Defaults.ConsensusTimeout != 30 {
		t.Errorf("ConsensusTimeout should be 30, got %d", cfg.Defaults.ConsensusTimeout)
	}
}

func TestLoad(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
}
