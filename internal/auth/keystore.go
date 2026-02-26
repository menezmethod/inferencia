// Package auth provides API key storage and validation.
//
// Keys are loaded from a text file (one key per line) or from
// a comma-separated environment variable. Lines starting with #
// are treated as comments. Empty lines are ignored.
package auth

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ErrInvalidKey is returned when an API key is not recognized.
var ErrInvalidKey = errors.New("invalid api key")

// KeyStore validates API keys against a set of known keys.
type KeyStore struct {
	mu   sync.RWMutex
	keys map[string]struct{}
}

// NewKeyStore creates a KeyStore and loads keys from the given file path.
// If the INFERENCIA_API_KEYS environment variable is set, those keys take
// precedence over the file.
func NewKeyStore(path string) (*KeyStore, error) {
	ks := &KeyStore{keys: make(map[string]struct{})}

	// Environment variable takes precedence.
	if env := os.Getenv("INFERENCIA_API_KEYS"); env != "" {
		for _, k := range strings.Split(env, ",") {
			if key := strings.TrimSpace(k); key != "" {
				ks.keys[key] = struct{}{}
			}
		}
		if len(ks.keys) == 0 {
			return nil, errors.New("INFERENCIA_API_KEYS is set but contains no valid keys")
		}
		return ks, nil
	}

	// Fall back to file.
	if path == "" {
		return nil, errors.New("no keys file path provided and INFERENCIA_API_KEYS is not set")
	}

	if err := ks.loadFile(path); err != nil {
		return nil, fmt.Errorf("load keys file: %w", err)
	}

	if len(ks.keys) == 0 {
		return nil, fmt.Errorf("keys file %q contains no valid keys", path)
	}

	return ks, nil
}

// Validate checks whether the given key is authorized.
func (ks *KeyStore) Validate(key string) error {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	if _, ok := ks.keys[key]; !ok {
		return ErrInvalidKey
	}
	return nil
}

// Count returns the number of loaded keys.
func (ks *KeyStore) Count() int {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return len(ks.keys)
}

// loadFile reads keys from a text file, one per line.
func (ks *KeyStore) loadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ks.keys[line] = struct{}{}
	}
	return scanner.Err()
}
