package apierror

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Error", func() {
	It("implements error interface with message", func() {
		e := InvalidRequest("bad input")
		Expect(e.Error()).To(Equal("bad input"))
	})
})

var _ = Describe("Write", func() {
	It("writes JSON with status and OpenAI envelope", func() {
		e := InvalidRequest("invalid JSON")
		rec := httptest.NewRecorder()
		Write(rec, e)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/json"))
		var body struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		Expect(json.NewDecoder(rec.Body).Decode(&body)).NotTo(HaveOccurred())
		Expect(body.Error.Message).To(Equal("invalid JSON"))
		Expect(body.Error.Type).To(Equal(TypeInvalidRequest))
	})
})

var _ = Describe("InvalidRequest", func() {
	It("returns 400 with type invalid_request_error", func() {
		e := InvalidRequest("missing model")
		Expect(e.Status).To(Equal(http.StatusBadRequest))
		Expect(e.Type).To(Equal(TypeInvalidRequest))
		Expect(e.Message).To(Equal("missing model"))
	})
})

var _ = Describe("InvalidParam", func() {
	It("returns 400 with param set", func() {
		e := InvalidParam("model", "model is required")
		Expect(e.Status).To(Equal(http.StatusBadRequest))
		Expect(e.Type).To(Equal(TypeInvalidRequest))
		Expect(e.Param).To(Equal("model"))
		Expect(e.Message).To(Equal("model is required"))
	})
})

var _ = Describe("Unauthorized", func() {
	It("returns 401 with invalid_api_key code", func() {
		e := Unauthorized("Invalid API key.")
		Expect(e.Status).To(Equal(http.StatusUnauthorized))
		Expect(e.Type).To(Equal(TypeAuthentication))
		Expect(e.Code).To(Equal("invalid_api_key"))
	})
})

var _ = Describe("RateLimited", func() {
	It("returns 429 with rate_limit_exceeded code", func() {
		e := RateLimited()
		Expect(e.Status).To(Equal(http.StatusTooManyRequests))
		Expect(e.Type).To(Equal(TypeRateLimit))
		Expect(e.Code).To(Equal("rate_limit_exceeded"))
	})
})

var _ = Describe("BackendUnavailable", func() {
	It("returns 503 with backend name in message", func() {
		e := BackendUnavailable("mlx")
		Expect(e.Status).To(Equal(http.StatusServiceUnavailable))
		Expect(e.Type).To(Equal(TypeBackendDown))
		Expect(e.Code).To(Equal("backend_unavailable"))
		Expect(e.Message).To(ContainSubstring("mlx"))
	})
})

var _ = Describe("Internal", func() {
	It("returns 500 with type server_error", func() {
		e := Internal("unexpected failure")
		Expect(e.Status).To(Equal(http.StatusInternalServerError))
		Expect(e.Type).To(Equal(TypeServer))
		Expect(e.Message).To(Equal("unexpected failure"))
	})
})
