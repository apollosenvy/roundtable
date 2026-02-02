// internal/config/config.go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ModelConfig struct {
	Enabled      bool   `yaml:"enabled"`
	CLIPath      string `yaml:"cli_path,omitempty"`
	APIKey       string `yaml:"api_key,omitempty"`
	DefaultModel string `yaml:"default_model,omitempty"`
}

type Config struct {
	Models struct {
		Claude ModelConfig `yaml:"claude"`
		Gemini ModelConfig `yaml:"gemini"`
		GPT    ModelConfig `yaml:"gpt"`
		Grok   ModelConfig `yaml:"grok"`
	} `yaml:"models"`
	Defaults struct {
		AutoDebate       bool `yaml:"auto_debate"`
		ConsensusTimeout int  `yaml:"consensus_timeout"`
		ModelTimeout     int  `yaml:"model_timeout"`
		RetryAttempts    int  `yaml:"retry_attempts"`
		RetryDelay       int  `yaml:"retry_delay"` // milliseconds
	} `yaml:"defaults"`
}

func Load() (*Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.ExpandEnv("$HOME/.config")
	}

	path := filepath.Join(configDir, "roundtable", "config.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		// Return defaults if no config file
		return defaultConfig(), nil
	}

	// Expand environment variables in config
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}

	// Apply defaults for unset values
	applyDefaults(&cfg)

	return &cfg, nil
}

func defaultConfig() *Config {
	cfg := &Config{}
	cfg.Models.Claude.Enabled = true
	cfg.Models.Claude.CLIPath = "claude"
	cfg.Models.Claude.DefaultModel = "opus"
	cfg.Models.Gemini.Enabled = true
	cfg.Models.Gemini.CLIPath = "gemini"
	cfg.Models.GPT.Enabled = false
	cfg.Models.Grok.Enabled = false
	cfg.Defaults.AutoDebate = true
	cfg.Defaults.ConsensusTimeout = 30
	cfg.Defaults.ModelTimeout = 60
	cfg.Defaults.RetryAttempts = 3
	cfg.Defaults.RetryDelay = 1000 // 1 second
	return cfg
}

func applyDefaults(cfg *Config) {
	if cfg.Models.Claude.CLIPath == "" {
		cfg.Models.Claude.CLIPath = "claude"
	}
	if cfg.Models.Gemini.CLIPath == "" {
		cfg.Models.Gemini.CLIPath = "gemini"
	}
	if cfg.Defaults.ConsensusTimeout == 0 {
		cfg.Defaults.ConsensusTimeout = 30
	}
	if cfg.Defaults.ModelTimeout == 0 {
		cfg.Defaults.ModelTimeout = 60
	}
	if cfg.Defaults.RetryAttempts == 0 {
		cfg.Defaults.RetryAttempts = 3
	}
	if cfg.Defaults.RetryDelay == 0 {
		cfg.Defaults.RetryDelay = 1000
	}
}

func ConfigPath() string {
	configDir, _ := os.UserConfigDir()
	if configDir == "" {
		configDir = os.ExpandEnv("$HOME/.config")
	}
	return filepath.Join(configDir, "roundtable", "config.yaml")
}
