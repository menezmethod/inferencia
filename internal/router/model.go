// Package router provides capability-aware backend routing and selection.
package router

import "github.com/menezmethod/inferencia/internal/backend"

// Capability represents what kind of inference a backend supports.
type Capability int

const (
	// CapTTS indicates the backend supports text-to-speech.
	CapTTS Capability = iota
)

// BackendInfo describes a registered backend and its capabilities.
type BackendInfo struct {
	Name         string
	TTSBackend   backend.TTSBackend // TTS capable (may be nil)
	Capabilities []Capability
	Models       []ModelInfo // advertised models, used for scoring
}

// ModelInfo describes a single model exposed by a backend.
type ModelInfo struct {
	ID       string
	Provider string
	Kind     Capability
}
