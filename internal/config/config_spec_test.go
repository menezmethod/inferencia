package config

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Defaults", func() {
	It("sets server port 8080 and host 127.0.0.1", func() {
		cfg := Defaults()
		Expect(cfg.Server.Port).To(Equal(8080))
		Expect(cfg.Server.Host).To(Equal("127.0.0.1"))
	})

	It("includes one MLX backend", func() {
		cfg := Defaults()
		Expect(cfg.Backends).To(HaveLen(1))
		Expect(cfg.Backends[0].Type).To(Equal("mlx"))
	})
})

var _ = Describe("Load", func() {
	When("loading from a valid file", func() {
		It("overrides defaults with file values", func() {
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
			path := filepath.Join(GinkgoT().TempDir(), "config.yaml")
			Expect(os.WriteFile(path, []byte(content), 0644)).NotTo(HaveOccurred())

			cfg, err := Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Server.Host).To(Equal("0.0.0.0"))
			Expect(cfg.Server.Port).To(Equal(9090))
			Expect(cfg.Server.WriteTimeout).To(Equal(60 * time.Second))
			Expect(cfg.Backends[0].Name).To(Equal("test-mlx"))
			Expect(cfg.RateLimit.Burst).To(Equal(10))
			Expect(cfg.Log.Level).To(Equal("debug"))
		})
	})

	When("environment variables are set", func() {
		It("overrides file values with env", func() {
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
			path := filepath.Join(GinkgoT().TempDir(), "config.yaml")
			Expect(os.WriteFile(path, []byte(content), 0644)).NotTo(HaveOccurred())

			os.Setenv("INFERENCIA_PORT", "3000")
			os.Setenv("INFERENCIA_LOG_LEVEL", "debug")
			defer os.Unsetenv("INFERENCIA_PORT")
			defer os.Unsetenv("INFERENCIA_LOG_LEVEL")

			cfg, err := Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Server.Port).To(Equal(3000))
			Expect(cfg.Log.Level).To(Equal("debug"))
		})
	})

	When("INFERENCIA_BACKEND_URL is set", func() {
		It("overrides the first backend URL", func() {
			os.Setenv("INFERENCIA_BACKEND_URL", "http://192.168.0.x:11973")
			defer os.Unsetenv("INFERENCIA_BACKEND_URL")

			cfg, err := Load("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Backends).NotTo(BeEmpty())
			Expect(cfg.Backends[0].URL).To(Equal("http://192.168.0.x:11973"))
		})
	})
})

var _ = Describe("Validation", func() {
	When("config is valid", func() {
		It("returns no error", func() {
			cfg := Defaults()
			Expect(validate(cfg)).NotTo(HaveOccurred())
		})
	})

	When("port is zero", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.Server.Port = 0
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})

	When("port is too high", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.Server.Port = 70000
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})

	When("there are no backends", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.Backends = nil
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})

	When("backend has no name", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.Backends[0].Name = ""
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})

	When("log level is invalid", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.Log.Level = "verbose"
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})

	When("log format is invalid", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.Log.Format = "xml"
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})

	When("rate limit rps is zero", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.RateLimit.RequestsPerSecond = 0
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})

	When("OTel is enabled but endpoint is empty", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.Observability.OTelEnabled = true
			cfg.Observability.OTelEndpoint = ""
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})

	When("cloud_format is invalid", func() {
		It("returns an error", func() {
			cfg := Defaults()
			cfg.Log.CloudFormat = "aws"
			Expect(validate(cfg)).To(HaveOccurred())
		})
	})
})

var _ = Describe("Server Addr", func() {
	It("returns host:port", func() {
		s := Server{Host: "0.0.0.0", Port: 3000}
		Expect(s.Addr()).To(Equal("0.0.0.0:3000"))
	})
})
