package watchdog_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/router"
	"github.com/menezmethod/inferencia/internal/watchdog"
)

var _ = Describe("Watchdog", func() {
	var (
		logger *slog.Logger
		reg    *backend.Registry
		ttsReg *router.Registry
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
		reg = backend.NewRegistry()
		ttsReg = router.NewRegistry()
	})

	Describe("healthy backends", func() {
		It("reports healthy after probing a reachable backend", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			reg.Register(backend.NewOllama("test-ollama", srv.URL, 5*time.Second))

			wd := watchdog.New(watchdog.Config{
				Interval:       50 * time.Millisecond,
				FailThreshold:  3,
				RequestTimeout: 2 * time.Second,
			}, reg, ttsReg, logger)
			wd.Start()
			defer wd.Stop()

			Eventually(func() bool {
				return wd.IsHealthy("test-ollama")
			}, 500*time.Millisecond, 25*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("degraded after consecutive failures", func() {
		It("marks backend degraded after fail threshold", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			reg.Register(backend.NewOllama("bad-backend", srv.URL, 5*time.Second))

			wd := watchdog.New(watchdog.Config{
				Interval:       25 * time.Millisecond,
				FailThreshold:  2,
				RequestTimeout: 2 * time.Second,
			}, reg, ttsReg, logger)
			wd.Start()
			defer wd.Stop()

			Eventually(func() bool {
				return !wd.IsHealthy("bad-backend")
			}, 500*time.Millisecond, 25*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("recovery", func() {
		It("recovers after backend comes back online", func() {
			var up atomic.Bool
			up.Store(true)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if up.Load() {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer srv.Close()

			reg.Register(backend.NewOllama("flaky", srv.URL, 5*time.Second))

			wd := watchdog.New(watchdog.Config{
				Interval:       25 * time.Millisecond,
				FailThreshold:  2,
				RequestTimeout: 2 * time.Second,
			}, reg, ttsReg, logger)
			wd.Start()
			defer wd.Stop()

			Eventually(func() bool {
				return wd.IsHealthy("flaky")
			}, 500*time.Millisecond, 25*time.Millisecond).Should(BeTrue())

			up.Store(false)
			Eventually(func() bool {
				return !wd.IsHealthy("flaky")
			}, 500*time.Millisecond, 25*time.Millisecond).Should(BeTrue())

			up.Store(true)
			Eventually(func() bool {
				return wd.IsHealthy("flaky")
			}, 500*time.Millisecond, 25*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("TTS backend probing", func() {
		It("probes TTS backends too", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			ttsBackend := backend.NewTTSHTTP("kokoro", srv.URL, 5*time.Second)
			ttsReg.Register(router.BackendInfo{
				Name:         "kokoro",
				TTSBackend:   ttsBackend,
				Capabilities: []router.Capability{router.CapTTS},
				Models:       []router.ModelInfo{{ID: "kokoro", Kind: router.CapTTS}},
			})

			wd := watchdog.New(watchdog.Config{
				Interval:       50 * time.Millisecond,
				FailThreshold:  3,
				RequestTimeout: 2 * time.Second,
			}, reg, ttsReg, logger)
			wd.Start()
			defer wd.Stop()

			Eventually(func() bool {
				return wd.IsHealthy("kokoro")
			}, 500*time.Millisecond, 25*time.Millisecond).Should(BeTrue())
		})
	})
})
