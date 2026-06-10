package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/identity"
)

// Config holds server settings loaded from IDLEFARM_* environment variables.
type Config struct {
	ListenHost          string
	ListenPort          int
	HostKeyPath         string
	IdleTimeout         time.Duration
	MaxSessionsPerKey   int
	MaxConnections      int
	DefaultSlot         string
	LogLevel            string
	LogFormat           string
	RateLimitPerSecond  float64
	RateLimitBurst      int
	RateLimitMaxEntries int
	DBPath              string
	AutosaveInterval    time.Duration
	SessionPolicy       string // "takeover" or "refuse"
	DataDir             string // content override dir; empty = embedded
}

// Load reads configuration from the environment with documented defaults.
func Load() (Config, error) {
	var err error
	cfg := Config{
		ListenHost:    envOr("IDLEFARM_LISTEN_HOST", "0.0.0.0"),
		HostKeyPath:   envOr("IDLEFARM_HOST_KEY_PATH", "var/ssh_host_key"),
		DefaultSlot:   envOr("IDLEFARM_DEFAULT_SLOT", "default"),
		LogLevel:      envOr("IDLEFARM_LOG_LEVEL", "info"),
		LogFormat:     envOr("IDLEFARM_LOG_FORMAT", "text"),
		DBPath:        envOr("IDLEFARM_DB_PATH", "var/idlefarm.db"),
		SessionPolicy: envOr("IDLEFARM_SESSION_POLICY", "takeover"),
		DataDir:       os.Getenv("IDLEFARM_DATA_DIR"),
	}
	if cfg.ListenPort, err = envIntOr("IDLEFARM_LISTEN_PORT", 22); err != nil {
		return Config{}, err
	}
	if cfg.IdleTimeout, err = envDurationOr("IDLEFARM_IDLE_TIMEOUT", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.MaxSessionsPerKey, err = envIntOr("IDLEFARM_MAX_SESSIONS_PER_KEY", 2); err != nil {
		return Config{}, err
	}
	if cfg.MaxConnections, err = envIntOr("IDLEFARM_MAX_CONNECTIONS", 100); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitPerSecond, err = envFloatOr("IDLEFARM_RATE_LIMIT_PER_SECOND", 2); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitBurst, err = envIntOr("IDLEFARM_RATE_LIMIT_BURST", 5); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitMaxEntries, err = envIntOr("IDLEFARM_RATE_LIMIT_MAX_IPS", 1000); err != nil {
		return Config{}, err
	}
	if cfg.AutosaveInterval, err = envDurationOr("IDLEFARM_AUTOSAVE_INTERVAL", 30*time.Second); err != nil {
		return Config{}, err
	}

	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return Config{}, fmt.Errorf("IDLEFARM_LISTEN_PORT must be 1-65535, got %d", cfg.ListenPort)
	}
	if cfg.MaxSessionsPerKey < 1 {
		return Config{}, fmt.Errorf("IDLEFARM_MAX_SESSIONS_PER_KEY must be at least 1")
	}
	if cfg.MaxConnections < 1 {
		return Config{}, fmt.Errorf("IDLEFARM_MAX_CONNECTIONS must be at least 1")
	}
	if cfg.RateLimitPerSecond <= 0 {
		return Config{}, fmt.Errorf("IDLEFARM_RATE_LIMIT_PER_SECOND must be positive")
	}
	if cfg.RateLimitBurst < 1 {
		return Config{}, fmt.Errorf("IDLEFARM_RATE_LIMIT_BURST must be at least 1")
	}
	if cfg.IdleTimeout < 0 {
		return Config{}, fmt.Errorf("IDLEFARM_IDLE_TIMEOUT must be zero or positive")
	}
	slot := identity.SanitizeSlot(cfg.DefaultSlot)
	if slot == "" {
		return Config{}, fmt.Errorf("IDLEFARM_DEFAULT_SLOT must sanitize to 1-32 characters [a-z0-9_-]")
	}
	cfg.DefaultSlot = slot

	if cfg.AutosaveInterval < time.Second {
		return Config{}, fmt.Errorf("IDLEFARM_AUTOSAVE_INTERVAL must be at least 1s")
	}
	if cfg.SessionPolicy != "takeover" && cfg.SessionPolicy != "refuse" {
		return Config{}, fmt.Errorf("IDLEFARM_SESSION_POLICY must be %q or %q, got %q", "takeover", "refuse", cfg.SessionPolicy)
	}
	if cfg.DBPath == "" {
		return Config{}, fmt.Errorf("IDLEFARM_DB_PATH must not be empty")
	}

	return cfg, nil
}

func (c Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.ListenHost, c.ListenPort)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", key, v)
	}
	return n, nil
}

func envFloatOr(key string, fallback float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid float %q", key, v)
	}
	return f, nil
}

func envDurationOr(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q", key, v)
	}
	return d, nil
}
