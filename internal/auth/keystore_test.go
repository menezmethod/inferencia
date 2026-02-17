package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewKeyStoreFromFile(t *testing.T) {
	content := `# This is a comment
sk-key-one
sk-key-two

# Another comment
sk-key-three
`
	path := filepath.Join(t.TempDir(), "keys.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ks, err := NewKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if ks.Count() != 3 {
		t.Errorf("Count() = %d, want 3", ks.Count())
	}
}

func TestNewKeyStoreFromEnv(t *testing.T) {
	t.Setenv("INFERENCIA_API_KEYS", "sk-env-one, sk-env-two")

	ks, err := NewKeyStore("")
	if err != nil {
		t.Fatal(err)
	}

	if ks.Count() != 2 {
		t.Errorf("Count() = %d, want 2", ks.Count())
	}
}

func TestValidate(t *testing.T) {
	t.Setenv("INFERENCIA_API_KEYS", "sk-valid-key")

	ks, err := NewKeyStore("")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "sk-valid-key", false},
		{"invalid key", "sk-wrong-key", true},
		{"empty key", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ks.Validate(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestEmptyKeysFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(path, []byte("# only comments\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewKeyStore(path)
	if err == nil {
		t.Error("expected error for empty keys file, got nil")
	}
}

func TestMissingFile(t *testing.T) {
	_, err := NewKeyStore("/nonexistent/keys.txt")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
