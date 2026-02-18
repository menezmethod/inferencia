package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/menezmethod/inferencia/internal/backend"
)

func TestModels(t *testing.T) {
	mock := &mockBackend{
		modelsResp: &backend.ModelsResponse{
			Object: "list",
			Data: []backend.Model{
				{ID: "gpt-oss-20b", Object: "model", OwnedBy: "local"},
			},
		},
	}
	reg := newTestRegistry(mock)
	handler := Models(reg, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp backend.ModelsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("models count = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].ID != "gpt-oss-20b" {
		t.Errorf("model ID = %q, want gpt-oss-20b", resp.Data[0].ID)
	}
}

func TestModelsBackendDown(t *testing.T) {
	mock := &mockBackend{
		modelsErr: errors.New("connection refused"),
	}
	reg := newTestRegistry(mock)
	handler := Models(reg, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
