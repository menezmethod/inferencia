// Package config handles loading and validating application configuration.
//
// Configuration is loaded from a YAML file with environment variable overrides.
// Environment variables use the INFERENCIA_ prefix (e.g., INFERENCIA_PORT).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the complete application configuration.
type Config struct {
	Server    Server    `yaml:"server"`
	Auth      Auth      `yaml:"auth"`
	Backends  []Backend `yaml:"backends"`
	RateLimit RateLimit `yaml:"ratelimit"`
	Log       Log       `yaml:"log"`
}

// Server configures the HTTP listener.
type Server struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// Auth configures API key authentication.
type Auth struct {
	KeysFile string `yaml:"keys_file"`
}

// Backend configures a single LLM backend.
type Backend struct {
	Name    string        `yaml:"name"`
	Type    string        `yaml:"type"`
	URL     string        `yaml:"url"`
	Timeout time.Duration `yaml:"timeout"`
}

// RateLimit configures the token bucket rate limiter.
type RateLimit struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
}

// Log configures structured logging.
type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		Server: Server{
			Host:         "127.0.0.1",
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
		},
		Auth: Auth{
			KeysFile: "./keys.txt",
		},
		Backends: []Backend{
			{
				Name:    "mlx",
				Type:    "mlx",
				URL:     "http://localhost:11973",
				Timeout: 60 * time.Second,
			},
		},
		RateLimit: RateLimit{
			RequestsPerSecond: 10,
			Burst:             20,
		},
		Log: Log{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load reads configuration from the given YAML file path, then applies
// environment variable overrides. If path is empty, only defaults and
// environment variables are used.
func Load(path string) (Config, error) {
	cfg := Defaults()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config file: %w", err)
		}
	}

	applyEnvOverrides(&cfg)

	if err := validate(cfg); err != nil {
		return cfg, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// applyEnvOverrides reads INFERENCIA_* environment variables and overrides
// the corresponding config values.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("INFERENCIA_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("INFERENCIA_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("INFERENCIA_AUTH_KEYS_FILE"); v != "" {
		cfg.Auth.KeysFile = v
	}
	if v := os.Getenv("INFERENCIA_LOG_LEVEL"); v != "" {
		cfg.Log.Level = strings.ToLower(v)
	}
	if v := os.Getenv("INFERENCIA_LOG_FORMAT"); v != "" {
		cfg.Log.Format = strings.ToLower(v)
	}
	if v := os.Getenv("INFERENCIA_RATELIMIT_RPS"); v != "" {
		if rps, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RateLimit.RequestsPerSecond = rps
		}
	}
	if v := os.Getenv("INFERENCIA_RATELIMIT_BURST"); v != "" {
		if burst, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.Burst = burst
		}
	}
	if v := os.Getenv("INFERENCIA_BACKEND_URL"); v != "" && len(cfg.Backends) > 0 {
		cfg.Backends[0].URL = strings.TrimSpace(v)
	}
}

// validate checks that the configuration is internally consistent.
func validate(cfg Config) error {
	var errs []error

	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("server.port must be between 1 and 65535, got %d", cfg.Server.Port))
	}
	if len(cfg.Backends) == 0 {
		errs = append(errs, errors.New("at least one backend must be configured"))
	}
	for i, b := range cfg.Backends {
		if b.Name == "" {
			errs = append(errs, fmt.Errorf("backends[%d].name is required", i))
		}
		if b.Type == "" {
			errs = append(errs, fmt.Errorf("backends[%d].type is required", i))
		}
		if b.URL == "" {
			errs = append(errs, fmt.Errorf("backends[%d].url is required", i))
		}
	}
	if cfg.RateLimit.RequestsPerSecond <= 0 {
		errs = append(errs, errors.New("ratelimit.requests_per_second must be positive"))
	}
	if cfg.RateLimit.Burst < 1 {
		errs = append(errs, errors.New("ratelimit.burst must be at least 1"))
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[cfg.Log.Level] {
		errs = append(errs, fmt.Errorf("log.level must be one of debug, info, warn, error; got %q", cfg.Log.Level))
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[cfg.Log.Format] {
		errs = append(errs, fmt.Errorf("log.format must be json or text; got %q", cfg.Log.Format))
	}

	return errors.Join(errs...)
}

// Addr returns the listen address as "host:port".
func (s Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
