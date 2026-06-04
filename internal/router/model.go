// Package router provides capability-aware backend routing and selection.
package router

import "github.com/menezmethod/inferencia/internal/backend"

// Capability represents what kind of inference a backend supports.
type Capability int

const (
	// CapChat indicates the backend supports chat completions.
	CapChat Capability = iota
	// CapEmbed indicates the backend supports embeddings.
	CapEmbed
	// CapTTS indicates the backend supports text-to-speech.
	CapTTS
)

// String returns the human-readable name of the capability.
func (c Capability) String() string {
	switch c {
	case CapChat:
		return "chat"
	case CapEmbed:
		return "embed"
	case CapTTS:
		return "tts"
	default:
		return "unknown"
	}
}

// BackendInfo describes a registered backend and its capabilities.
type BackendInfo struct {
	Name         string
	Backend      backend.Backend   // chat/embed capable (may be nil)
	TTSBackend   backend.TTSBackend // TTS capable (may be nil)
	Capabilities []Capability
	Models       []ModelInfo
}

// ModelInfo describes a single model exposed by a backend.
type ModelInfo struct {
	ID       string
	Provider string
	Kind     Capability
}

// ModelRoute maps a model name to a backend and capability.
type ModelRoute struct {
	Model      string
	Capability Capability
	BackendName string
}
