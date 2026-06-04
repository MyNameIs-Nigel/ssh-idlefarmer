package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
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
}

// Load reads configuration from the environment with documented defaults.
func Load() (Config, error) {
	cfg := Config{
		ListenHost:          envOr("IDLEFARM_LISTEN_HOST", "0.0.0.0"),
		ListenPort:          envIntOr("IDLEFARM_LISTEN_PORT", 22),
		HostKeyPath:         envOr("IDLEFARM_HOST_KEY_PATH", "var/ssh_host_key"),
		IdleTimeout:         envDurationOr("IDLEFARM_IDLE_TIMEOUT", 30*time.Minute),
		MaxSessionsPerKey:   envIntOr("IDLEFARM_MAX_SESSIONS_PER_KEY", 2),
		MaxConnections:      envIntOr("IDLEFARM_MAX_CONNECTIONS", 100),
		DefaultSlot:         envOr("IDLEFARM_DEFAULT_SLOT", "default"),
		LogLevel:            envOr("IDLEFARM_LOG_LEVEL", "info"),
		LogFormat:           envOr("IDLEFARM_LOG_FORMAT", "text"),
		RateLimitPerSecond:  envFloatOr("IDLEFARM_RATE_LIMIT_PER_SECOND", 2),
		RateLimitBurst:      envIntOr("IDLEFARM_RATE_LIMIT_BURST", 5),
		RateLimitMaxEntries: envIntOr("IDLEFARM_RATE_LIMIT_MAX_IPS", 1000),
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

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloatOr(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
