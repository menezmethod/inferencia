// Package integration provides helpers to start and stop the inferencia app for integration tests.
package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	integrationPort = "18080"
	healthPath      = "/health"
)

// StartApp builds the binary (if needed), starts the app with test env, and waits for /health.
// Returns baseURL (e.g. "http://127.0.0.1:18080") and a cleanup function that must be called to stop the process.
func StartApp() (baseURL string, cleanup func(), err error) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return "", nil, fmt.Errorf("find repo root: %w", err)
	}

	// Ensure OpenAPI spec is present for embed.
	specSrc := filepath.Join(repoRoot, "docs", "openapi.yaml")
	specDst := filepath.Join(repoRoot, "internal", "openapi", "spec.yaml")
	if _, statErr := os.Stat(specDst); statErr != nil {
		if err := copyFile(specSrc, specDst); err != nil {
			return "", nil, fmt.Errorf("copy openapi spec: %w", err)
		}
	}

	binaryName := "inferencia_integration_test"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(repoRoot, binaryName)

	// Build the binary.
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/inferencia")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
	if out, buildErr := build.CombinedOutput(); buildErr != nil {
		return "", nil, fmt.Errorf("build binary: %w\n%s", buildErr, out)
	}

	// Start the app.
	cmd := exec.Command(binaryPath)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"INFERENCIA_HOST=127.0.0.1",
		"INFERENCIA_PORT="+integrationPort,
		"INFERENCIA_API_KEYS=sk-integration-test",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("start app: %w", err)
	}

	baseURL = "http://127.0.0.1:" + integrationPort
	cleanup = func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.Remove(binaryPath)
	}

	// Wait for server to be ready.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := waitForHealth(ctx, baseURL); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("wait for health: %w", err)
	}

	return baseURL, cleanup, nil
}

func findRepoRoot() (string, error) {
	if root := os.Getenv("INTEGRATION_REPO_ROOT"); root != "" {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root, nil
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	startDir := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", startDir)
		}
		dir = parent
	}
}

func waitForHealth(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+healthPath, nil)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// GetIntegrationPort returns the port used by the integration server (for Postman/Newman env).
func GetIntegrationPort() string { return integrationPort }

// GetIntegrationBaseURL returns the default base URL when server is running (host:port, no scheme).
func GetIntegrationBaseURL() string { return "127.0.0.1:" + integrationPort }

// IsRunning returns true if the integration server responds on the given baseURL.
func IsRunning(baseURL string) bool {
	resp, err := http.Get(strings.TrimSuffix(baseURL, "/") + healthPath)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
