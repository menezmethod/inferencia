// Package version holds build-time version and commit (set via ldflags).
package version

// Version is the semantic version (e.g. "1.0.0"). Set at build: -ldflags "-X github.com/menezmethod/inferencia/internal/version.Version=..."
var Version = "dev"

// Commit is the git commit hash. Set at build: -ldflags "-X github.com/menezmethod/inferencia/internal/version.Commit=..."
var Commit = ""
