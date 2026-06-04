package router

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/menezmethod/inferencia/internal/backend"
)

var _ = Describe("Registry", func() {
	Describe("NewRegistry", func() {
		It("returns an empty registry", func() {
			reg := NewRegistry()
			Expect(reg).NotTo(BeNil())
			Expect(reg.Len()).To(Equal(0))
			Expect(reg.All()).To(BeEmpty())
		})
	})

	Describe("Register and Get", func() {
		It("registers a BackendInfo and retrieves it by name", func() {
			reg := NewRegistry()
			info := BackendInfo{
				Name: "test-backend",
				Capabilities: []Capability{CapChat, CapEmbed},
				Models: []ModelInfo{
					{ID: "llama3", Provider: "ollama", Kind: CapChat},
				},
			}
			reg.Register(info)

			got, ok := reg.Get("test-backend")
			Expect(ok).To(BeTrue())
			Expect(got.Name).To(Equal("test-backend"))
			Expect(got.Capabilities).To(ContainElement(CapChat))
			Expect(got.Capabilities).To(ContainElement(CapEmbed))
		})

		It("returns false for unknown name", func() {
			reg := NewRegistry()
			reg.Register(BackendInfo{Name: "known"})

			_, ok := reg.Get("unknown")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("All", func() {
		It("returns all registered backends", func() {
			reg := NewRegistry()
			reg.Register(BackendInfo{Name: "a"})
			reg.Register(BackendInfo{Name: "b"})

			all := reg.All()
			Expect(all).To(HaveLen(2))
			names := []string{all[0].Name, all[1].Name}
			Expect(names).To(ConsistOf("a", "b"))
		})
	})

	Describe("BackendsByCapability", func() {
		It("returns backends filtered by capability", func() {
			reg := NewRegistry()
			reg.Register(BackendInfo{
				Name:         "chat-bot",
				Capabilities: []Capability{CapChat},
			})
			reg.Register(BackendInfo{
				Name:         "tts-bot",
				Capabilities: []Capability{CapTTS},
			})
			reg.Register(BackendInfo{
				Name:         "hybrid",
				Capabilities: []Capability{CapChat, CapTTS},
			})

			ttsBackends := reg.BackendsByCapability(CapTTS)
			Expect(ttsBackends).To(HaveLen(2))
			ttsNames := []string{ttsBackends[0].Name, ttsBackends[1].Name}
			Expect(ttsNames).To(ConsistOf("tts-bot", "hybrid"))

			chatBackends := reg.BackendsByCapability(CapChat)
			Expect(chatBackends).To(HaveLen(2))
		})

		It("returns empty slice when no backends support the capability", func() {
			reg := NewRegistry()
			reg.Register(BackendInfo{
				Name:         "chat-bot",
				Capabilities: []Capability{CapChat},
			})

			embedBackends := reg.BackendsByCapability(CapEmbed)
			Expect(embedBackends).To(BeEmpty())
		})
	})
})

var _ = Describe("SelectBackend", func() {
	Describe("by capability", func() {
		It("returns the first backend that supports the capability when no model specified", func() {
			reg := NewRegistry()
			reg.Register(BackendInfo{
				Name:         "tts-one",
				TTSBackend:   &mockTTSBackend{name: "mock-tts"},
				Capabilities: []Capability{CapTTS},
			})
			reg.Register(BackendInfo{
				Name:         "tts-two",
				TTSBackend:   &mockTTSBackend{name: "mock-tts"},
				Capabilities: []Capability{CapTTS},
			})

			info, err := reg.SelectBackend(CapTTS, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Name).To(Equal("tts-one"))
		})
	})

	Describe("by capability and model", func() {
		It("prefers backends that advertise the matching model", func() {
			reg := NewRegistry()
			reg.Register(BackendInfo{
				Name:         "default",
				TTSBackend:   &mockTTSBackend{name: "mock-tts"},
				Capabilities: []Capability{CapTTS},
				Models: []ModelInfo{
					{ID: "kokoro", Provider: "kokoro", Kind: CapTTS},
				},
			})
			reg.Register(BackendInfo{
				Name:         "premium",
				TTSBackend:   &mockTTSBackend{name: "mock-tts"},
				Capabilities: []Capability{CapTTS},
				Models: []ModelInfo{
					{ID: "elevenlabs", Provider: "elevenlabs", Kind: CapTTS},
				},
			})

			info, err := reg.SelectBackend(CapTTS, "kokoro")
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Name).To(Equal("default"))
		})

		It("returns error when no backends support the capability", func() {
			reg := NewRegistry()
			reg.Register(BackendInfo{
				Name:         "chat-bot",
				Capabilities: []Capability{CapChat},
			})

			_, err := reg.SelectBackend(CapTTS, "")
			Expect(err).To(HaveOccurred())
		})
	})
})

// mockTTSBackend is a minimal TTSBackend for testing.
type mockTTSBackend struct {
	name string
}

func (m *mockTTSBackend) Name() string                                       { return m.name }
func (m *mockTTSBackend) Health(ctx context.Context) error                    { return nil }
func (m *mockTTSBackend) Synthesize(ctx context.Context, req backend.TTSRequest) (*backend.TTSResponse, error) {
	return &backend.TTSResponse{Audio: []byte{}, Format: "audio/wav"}, nil
}
func (m *mockTTSBackend) Voices(ctx context.Context) ([]backend.Voice, error) {
	return []backend.Voice{{ID: "default", Name: "Default"}}, nil
}
