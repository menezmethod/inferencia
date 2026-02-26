package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("default host = %q, want 127.0.0.1", cfg.Server.Host)
	}
	if len(cfg.Backends) != 1 {
		t.Fatalf("default backends count = %d, want 1", len(cfg.Backends))
	}
	if cfg.Backends[0].Type != "mlx" {
		t.Errorf("default backend type = %q, want mlx", cfg.Backends[0].Type)
	}
}

func TestLoadFromFile(t *testing.T) {
	content := `
server:
  host: "0.0.0.0"
  port: 9090
  read_timeout: 10s
  write_timeout: 60s
auth:
  keys_file: "/tmp/keys.txt"
backends:
  - name: "test-mlx"
    type: "mlx"
    url: "http://localhost:5555"
    timeout: 30s
ratelimit:
  requests_per_second: 5
  burst: 10
log:
  level: "debug"
  format: "text"
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("host = %q, want 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.WriteTimeout != 60*time.Second {
		t.Errorf("write_timeout = %v, want 60s", cfg.Server.WriteTimeout)
	}
	if cfg.Backends[0].Name != "test-mlx" {
		t.Errorf("backend name = %q, want test-mlx", cfg.Backends[0].Name)
	}
	if cfg.RateLimit.Burst != 10 {
		t.Errorf("burst = %d, want 10", cfg.RateLimit.Burst)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log level = %q, want debug", cfg.Log.Level)
	}
}

func TestEnvOverrides(t *testing.T) {
	content := `
server:
  port: 8080
backends:
  - name: "mlx"
    type: "mlx"
    url: "http://localhost:11973"
    timeout: 60s
ratelimit:
  requests_per_second: 10
  burst: 20
log:
  level: "info"
  format: "json"
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("INFERENCIA_PORT", "3000")
	t.Setenv("INFERENCIA_LOG_LEVEL", "debug")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Port != 3000 {
		t.Errorf("port = %d, want 3000 (env override)", cfg.Server.Port)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log level = %q, want debug (env override)", cfg.Log.Level)
	}
}

func TestEnvOverrideBackendURL(t *testing.T) {
	t.Setenv("INFERENCIA_BACKEND_URL", "http://192.168.0.50:11973")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Backends) == 0 {
		t.Fatal("expected default backend")
	}
	if cfg.Backends[0].URL != "http://192.168.0.50:11973" {
		t.Errorf("backend URL = %q, want http://192.168.0.50:11973 (INFERENCIA_BACKEND_URL)", cfg.Backends[0].URL)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid defaults",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "invalid port zero",
			modify:  func(c *Config) { c.Server.Port = 0 },
			wantErr: true,
		},
		{
			name:    "invalid port too high",
			modify:  func(c *Config) { c.Server.Port = 70000 },
			wantErr: true,
		},
		{
			name:    "no backends",
			modify:  func(c *Config) { c.Backends = nil },
			wantErr: true,
		},
		{
			name:    "backend missing name",
			modify:  func(c *Config) { c.Backends[0].Name = "" },
			wantErr: true,
		},
		{
			name:    "invalid log level",
			modify:  func(c *Config) { c.Log.Level = "verbose" },
			wantErr: true,
		},
		{
			name:    "invalid log format",
			modify:  func(c *Config) { c.Log.Format = "xml" },
			wantErr: true,
		},
		{
			name:    "zero rps",
			modify:  func(c *Config) { c.RateLimit.RequestsPerSecond = 0 },
			wantErr: true,
		},
		{
			name:    "otel enabled but no endpoint",
			modify:  func(c *Config) { c.Observability.OTelEnabled = true; c.Observability.OTelEndpoint = "" },
			wantErr: true,
		},
		{
			name:    "invalid cloud_format",
			modify:  func(c *Config) { c.Log.CloudFormat = "aws" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			tt.modify(&cfg)
			err := validate(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerAddr(t *testing.T) {
	s := Server{Host: "0.0.0.0", Port: 3000}
	if got := s.Addr(); got != "0.0.0.0:3000" {
		t.Errorf("Addr() = %q, want 0.0.0.0:3000", got)
	}
}
