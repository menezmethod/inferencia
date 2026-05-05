package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var baseURL string
var stopApp func()

var _ = BeforeSuite(func() {
	if u := os.Getenv("INTEGRATION_BASE_URL"); u != "" {
		baseURL = strings.TrimSuffix(u, "/")
		return
	}
	var err error
	baseURL, stopApp, err = StartApp()
	Expect(err).NotTo(HaveOccurred())
	Expect(baseURL).NotTo(BeEmpty())
})

var _ = AfterSuite(func() {
	if stopApp != nil {
		stopApp()
	}
})

var _ = Describe("Integration", func() {
	Describe("Unprotected endpoints", func() {
		It("GET /health returns 200 and status ok", func() {
			resp, err := http.Get(baseURL + "/health")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			var body map[string]string
			Expect(json.NewDecoder(resp.Body).Decode(&body)).NotTo(HaveOccurred())
			Expect(body["status"]).To(Equal("ok"))
			Expect(body).To(HaveKey("version"))
		})

		It("GET /version returns 200 and version in JSON", func() {
			resp, err := http.Get(baseURL + "/version")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))
			var body map[string]string
			Expect(json.NewDecoder(resp.Body).Decode(&body)).NotTo(HaveOccurred())
			Expect(body).To(HaveKey("version"))
		})

		It("GET /metrics returns 200 and Prometheus output", func() {
			resp, err := http.Get(baseURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, _ := io.ReadAll(resp.Body)
			Expect(string(body)).To(ContainSubstring("inferencia"))
		})

		It("GET /docs returns 200 and HTML with swagger-ui", func() {
			resp, err := http.Get(baseURL + "/docs")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/html"))
			body, _ := io.ReadAll(resp.Body)
			Expect(string(body)).To(ContainSubstring("swagger-ui"))
		})

		It("GET /openapi.yaml returns 200 and YAML", func() {
			resp, err := http.Get(baseURL + "/openapi.yaml")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/x-yaml"))
			body, _ := io.ReadAll(resp.Body)
			Expect(string(body)).To(ContainSubstring("openapi"))
		})
	})

	Describe("Protected endpoints require auth", func() {
		It("GET /v1/models without Authorization returns 401", func() {
			resp, err := http.Get(baseURL + "/v1/models")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("GET /v1/models with invalid key returns 401", func() {
			req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/models", nil)
			req.Header.Set("Authorization", "Bearer sk-wrong-key")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("GET /v1/models with valid key returns 503 (no backend) or 200", func() {
			req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/models", nil)
			req.Header.Set("Authorization", "Bearer sk-integration-test")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(BeElementOf(http.StatusOK, http.StatusServiceUnavailable))
		})
	})

	Describe("POST /v1/chat/completions", func() {
		It("without auth returns 401", func() {
			body := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
			resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("with valid auth and empty messages returns 400", func() {
			req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"test","messages":[]}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer sk-integration-test")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("POST /v1/embeddings", func() {
		It("without auth returns 401", func() {
			body := `{"model":"test","input":"hello"}`
			resp, err := http.Post(baseURL+"/v1/embeddings", "application/json", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("with valid auth and missing input returns 400", func() {
			req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/embeddings", bytes.NewBufferString(`{"model":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer sk-integration-test")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("GET /health/ready", func() {
		It("returns 200 when backends are healthy or 503 when none are", func() {
			resp, err := http.Get(baseURL + "/health/ready")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(BeElementOf(http.StatusOK, http.StatusServiceUnavailable))
		})
	})
})
