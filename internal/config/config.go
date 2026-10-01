// Package config handles loading and validating application configuration.
//
// Configuration is loaded from a YAML file with environment variable overrides.
// Environment variables use the INFERENCIA_ prefix (e.g., INFERENCIA_PORT).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the complete application configuration.
type Config struct {
	Server        Server        `yaml:"server"`
	Auth          Auth          `yaml:"auth"`
	Backends      []Backend     `yaml:"backends"`
	TTSBackends   []TTSBackend  `yaml:"tts_backends"`
	Decisions     []Decision    `yaml:"decisions"`
	RateLimit     RateLimit     `yaml:"ratelimit"`
	Log           Log           `yaml:"log"`
	Observability Observability `yaml:"observability"`
	Watchdog      Watchdog      `yaml:"watchdog"`
	Routing       Routing       `yaml:"routing"`
}

// Decision configures a System One decision engine (TypeSafe Jev, Laya).
// Both speak the same /v1/systemone protocol; url and API key are all that differ.
type Decision struct {
	Name      string   `yaml:"name"`
	URL       string   `yaml:"url"`
	APIKeyEnv string   `yaml:"api_key_env,omitempty"` // env var holding the bearer key (Jev requires one)
	Models    []string `yaml:"models,omitempty"`      // advertised model IDs for discovery and routing
}

// Routing configures virtual role models for task-aware routing.
type Routing struct {
	DefaultRole string          `yaml:"default_role"`
	Roles       map[string]Role `yaml:"roles"`
	Auto        Auto            `yaml:"auto"`
}

// Auto configures decision-powered automatic tier selection. When set, a chat
// request for the "auto" role asks the decision engine which configured role
// should handle it (local-first: name the local Laya engine first).
//
// Policies are named variants over the same planner, selected with
// "auto:quality", "auto:economy", "auto:free" (bare "auto" = balanced).
type Auto struct {
	Decision      string                `yaml:"decision"` // name of a decisions[] entry
	Model         string                `yaml:"model,omitempty"`
	Question      string                `yaml:"question,omitempty"`
	Criteria      map[string]string     `yaml:"criteria,omitempty"`       // role -> when to use it
	MinConfidence float64               `yaml:"min_confidence,omitempty"` // abstain below this confidence
	Policies      map[string]AutoPolicy `yaml:"policies,omitempty"`       // quality | balanced | economy | free
}

// AutoPolicy biases the planner without changing the underlying mechanism:
// different criteria, confidence floor, fallback role, or local-only constraint.
type AutoPolicy struct {
	Criteria      map[string]string `yaml:"criteria,omitempty"`
	MinConfidence float64           `yaml:"min_confidence,omitempty"`
	Fallback      string            `yaml:"fallback,omitempty"`
	LocalOnly     bool              `yaml:"local_only,omitempty"` // free/local backends only (ollama, mlx)
}

// Role maps a virtual model name (e.g. inferencia-planner) to an ordered
// list of concrete upstream model IDs plus the backend serving them.
type Role struct {
	Backend string   `yaml:"backend"`
	Models  []string `yaml:"models"`
}

// Watchdog configures the background health-check loop.
type Watchdog struct {
	Interval       time.Duration `yaml:"interval"`
	FailThreshold  int           `yaml:"fail_threshold"`
	RequestTimeout time.Duration `yaml:"request_timeout"`
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
	Name          string        `yaml:"name"`
	Type          string        `yaml:"type"` // ollama | mlx | commandcode
	URL           string        `yaml:"url"`
	APIKeyEnv     string        `yaml:"api_key_env,omitempty"` // env var holding the upstream API key (commandcode)
	Timeout       time.Duration `yaml:"timeout"`               // inference timeout (chat, embeddings); 0 disables client timeout
	HealthTimeout time.Duration `yaml:"health_timeout"`        // health probes and model listing
}

// TTSBackend configures a single TTS backend.
type TTSBackend struct {
	Name    string        `yaml:"name"`
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
	Level       string `yaml:"level"`
	Format      string `yaml:"format"`
	CloudFormat string `yaml:"cloud_format"` // "gcp" | "gcp_with_resource" | "" (disabled)
}

// Observability configures optional OpenTelemetry and cloud logging.
type Observability struct {
	OTelEnabled     bool   `yaml:"otel_enabled"`
	OTelEndpoint    string `yaml:"otel_endpoint"` // OTLP HTTP endpoint, e.g. http://localhost:4318/v1/traces
	OTelServiceName string `yaml:"otel_service_name"`
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
				Name:          "ollama",
				Type:          "ollama",
				URL:           "http://localhost:11434",
				Timeout:       300 * time.Second,
				HealthTimeout: 5 * time.Second,
			},
		},
		RateLimit: RateLimit{
			RequestsPerSecond: 10,
			Burst:             20,
		},
		Log: Log{
			Level:       "info",
			Format:      "json",
			CloudFormat: "",
		},
		Observability: Observability{
			OTelEnabled:     false,
			OTelEndpoint:    "",
			OTelServiceName: "inferencia",
		},
		Watchdog: Watchdog{
			Interval:       30 * time.Second,
			FailThreshold:  3,
			RequestTimeout: 5 * time.Second,
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
		} else {
			slog.Warn("invalid INFERENCIA_PORT, using default", "value", v, "err", err)
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
	if v := os.Getenv("INFERENCIA_LOG_CLOUD_FORMAT"); v != "" {
		cfg.Log.CloudFormat = strings.ToLower(strings.TrimSpace(v))
	}
	if v := os.Getenv("INFERENCIA_OTEL_ENABLED"); v != "" {
		cfg.Observability.OTelEnabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("INFERENCIA_OTEL_ENDPOINT"); v != "" {
		cfg.Observability.OTelEndpoint = strings.TrimSpace(v)
	}
	if v := os.Getenv("INFERENCIA_OTEL_SERVICE_NAME"); v != "" {
		cfg.Observability.OTelServiceName = strings.TrimSpace(v)
	}
	if v := os.Getenv("INFERENCIA_RATELIMIT_RPS"); v != "" {
		if rps, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RateLimit.RequestsPerSecond = rps
		} else {
			slog.Warn("invalid INFERENCIA_RATELIMIT_RPS, using default", "value", v, "err", err)
		}
	}
	if v := os.Getenv("INFERENCIA_RATELIMIT_BURST"); v != "" {
		if burst, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.Burst = burst
		} else {
			slog.Warn("invalid INFERENCIA_RATELIMIT_BURST, using default", "value", v, "err", err)
		}
	}
	if v := os.Getenv("INFERENCIA_BACKEND_URL"); v != "" && len(cfg.Backends) > 0 {
		cfg.Backends[0].URL = strings.TrimSpace(v)
	}
	if v := os.Getenv("INFERENCIA_ROUTING_DEFAULT_ROLE"); v != "" {
		cfg.Routing.DefaultRole = strings.TrimSpace(v)
	}
	if v := os.Getenv("INFERENCIA_COMMANDCODE_URL"); v != "" {
		for i := range cfg.Backends {
			if cfg.Backends[i].Type == "commandcode" && cfg.Backends[i].URL == "" {
				cfg.Backends[i].URL = strings.TrimSpace(v)
			}
		}
	}

	// TTS backend env vars: one canonical name per hosted TTS server.
	for env, name := range map[string]string{
		"INFERENCIA_KOKORO_URL":     "kokoro",
		"INFERENCIA_CHATTERBOX_URL": "chatterbox",
		"INFERENCIA_MISOTTS_URL":    "misotts",
		"INFERENCIA_ELEVENLABS_URL": "elevenlabs",
	} {
		if v := os.Getenv(env); v != "" && !ttsBackendExists(cfg.TTSBackends, name) {
			cfg.TTSBackends = append(cfg.TTSBackends, TTSBackend{
				Name:    name,
				URL:     strings.TrimSpace(v),
				Timeout: 30 * time.Second,
			})
		}
	}

	// Watchdog env vars.
	if v := os.Getenv("INFERENCIA_WATCHDOG_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Watchdog.Interval = d
		} else {
			slog.Warn("invalid INFERENCIA_WATCHDOG_INTERVAL, using default", "value", v, "err", err)
		}
	}
	if v := os.Getenv("INFERENCIA_WATCHDOG_FAIL_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Watchdog.FailThreshold = n
		} else if err != nil {
			slog.Warn("invalid INFERENCIA_WATCHDOG_FAIL_THRESHOLD, using default", "value", v, "err", err)
		} else {
			slog.Warn("invalid INFERENCIA_WATCHDOG_FAIL_THRESHOLD, must be positive, using default", "value", v)
		}
	}
	if v := os.Getenv("INFERENCIA_WATCHDOG_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Watchdog.RequestTimeout = d
		} else {
			slog.Warn("invalid INFERENCIA_WATCHDOG_TIMEOUT, using default", "value", v, "err", err)
		}
	}
}

// ttsBackendExists checks whether a TTS backend with the given name is already
// registered in the slice. Used to prevent duplicates when env vars overlap
// with YAML configuration.
func ttsBackendExists(backends []TTSBackend, name string) bool {
	for _, b := range backends {
		if b.Name == name {
			return true
		}
	}
	return false
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
		switch b.Type {
		case "ollama", "mlx", "commandcode":
		default:
			errs = append(errs, fmt.Errorf("backends[%d].type must be one of ollama, mlx, commandcode; got %q", i, b.Type))
		}
		if b.URL == "" && b.Type != "commandcode" {
			errs = append(errs, fmt.Errorf("backends[%d].url is required", i))
		}
	}
	if cfg.Routing.DefaultRole != "" {
		role, ok := cfg.Routing.Roles[cfg.Routing.DefaultRole]
		if !ok {
			errs = append(errs, fmt.Errorf("routing.default_role %q is not defined in routing.roles", cfg.Routing.DefaultRole))
		} else if len(role.Models) == 0 {
			errs = append(errs, fmt.Errorf("routing.roles[%q].models must not be empty", cfg.Routing.DefaultRole))
		}
	}
	for name, role := range cfg.Routing.Roles {
		if len(role.Models) == 0 {
			errs = append(errs, fmt.Errorf("routing.roles[%q].models must not be empty", name))
		}
	}
	decisionNames := make(map[string]bool, len(cfg.Decisions))
	for i, d := range cfg.Decisions {
		if d.Name == "" {
			errs = append(errs, fmt.Errorf("decisions[%d].name is required", i))
		}
		if d.URL == "" {
			errs = append(errs, fmt.Errorf("decisions[%d].url is required", i))
		}
		if d.Name != "" {
			if decisionNames[d.Name] {
				errs = append(errs, fmt.Errorf("decisions[%d].name %q is duplicated", i, d.Name))
			}
			decisionNames[d.Name] = true
		}
	}
	if cfg.Routing.Auto.Decision != "" {
		if !decisionNames[cfg.Routing.Auto.Decision] {
			errs = append(errs, fmt.Errorf("routing.auto.decision %q is not defined in decisions", cfg.Routing.Auto.Decision))
		}
		for tier := range cfg.Routing.Auto.Criteria {
			if !roleDefined(cfg.Routing, tier) {
				errs = append(errs, fmt.Errorf("routing.auto.criteria[%q] does not match a defined role", tier))
			}
		}
		for name, policy := range cfg.Routing.Auto.Policies {
			if !validAutoPolicies[name] {
				errs = append(errs, fmt.Errorf("routing.auto.policies[%q] must be one of balanced, quality, economy, free", name))
			}
			if policy.Fallback != "" && !roleDefined(cfg.Routing, policy.Fallback) {
				errs = append(errs, fmt.Errorf("routing.auto.policies[%q].fallback %q does not match a defined role", name, policy.Fallback))
			}
			for tier := range policy.Criteria {
				if !roleDefined(cfg.Routing, tier) {
					errs = append(errs, fmt.Errorf("routing.auto.policies[%q].criteria[%q] does not match a defined role", name, tier))
				}
			}
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
	validCloudFormats := map[string]bool{"": true, "gcp": true, "gcp_with_resource": true}
	if !validCloudFormats[cfg.Log.CloudFormat] {
		errs = append(errs, fmt.Errorf("log.cloud_format must be empty, gcp, or gcp_with_resource; got %q", cfg.Log.CloudFormat))
	}
	if cfg.Observability.OTelEnabled && cfg.Observability.OTelEndpoint == "" {
		errs = append(errs, errors.New("observability.otel_endpoint is required when otel_enabled is true"))
	}

	return errors.Join(errs...)
}

// Addr returns the listen address as "host:port".
func (s Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// roleDefined reports whether a name matches a configured role, accepting both
// the bare ("worker") and prefixed ("inferencia-worker") forms.
func roleDefined(cfg Routing, name string) bool {
	if _, ok := cfg.Roles[name]; ok {
		return true
	}
	if _, ok := cfg.Roles["inferencia-"+name]; ok {
		return true
	}
	_, ok := cfg.Roles[strings.TrimPrefix(name, "inferencia-")]
	return ok
}

// validAutoPolicies lists the auto policy suffixes understood by the planner.
var validAutoPolicies = map[string]bool{
	"balanced": true,
	"quality":  true,
	"economy":  true,
	"free":     true,
}
