package decision

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client", func() {
	var received []byte

	newEngine := func(handler http.HandlerFunc) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received, _ = io.ReadAll(r.Body)
			handler(w, r)
		}))
		DeferCleanup(srv.Close)
		return srv
	}

	Describe("Do", func() {
		It("posts the raw body to /v1/systemone with a bearer key", func() {
			srv := newEngine(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/v1/systemone"))
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer sk-test"))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"model":"jev-1.13.0"}`))
			})
			c := New("jev", srv.URL, "sk-test")
			res, err := c.Do(context.Background(), []byte(`{"state":"hi"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Status).To(Equal(http.StatusOK))
			Expect(string(res.Body)).To(ContainSubstring("jev-1.13.0"))
		})

		It("relays non-2xx responses instead of failing", func() {
			srv := newEngine(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error":"bad question"}`))
			})
			c := New("laya", srv.URL, "")
			res, err := c.Do(context.Background(), []byte(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Status).To(Equal(http.StatusUnprocessableEntity))
			Expect(string(res.Body)).To(ContainSubstring("bad question"))
		})

		It("fails on transport errors", func() {
			c := New("laya", "http://127.0.0.1:1", "")
			_, err := c.Do(context.Background(), []byte(`{}`))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Pick", func() {
		It("asks one choice question and returns the tier with confidence", func() {
			srv := newEngine(func(w http.ResponseWriter, r *http.Request) {
				var req predictRequest
				Expect(json.Unmarshal(received, &req)).To(Succeed())
				Expect(req.State).To(Equal("Refactor the auth middleware"))
				Expect(req.Model).To(Equal("laya"))
				q := req.Questions["tier"]
				Expect(q.Type).To(Equal("choice"))
				Expect(q.Instructions).To(Equal("Which tier?"))
				Expect(q.Criteria).To(HaveKeyWithValue("worker", "routine work"))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"answers":{"tier":{"type":"choice","choice":"worker","confidence":0.91}}}`))
			})
			c := New("laya", srv.URL, "")
			choice, confidence, err := c.Pick(context.Background(), "Refactor the auth middleware", "Which tier?",
				"laya", map[string]string{"worker": "routine work", "planner": "deep work"}, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(choice).To(Equal("worker"))
			Expect(confidence).To(BeNumerically("~", 0.91, 0.001))
		})

		It("errors when criteria are empty", func() {
			c := New("laya", "http://127.0.0.1:1", "")
			_, _, err := c.Pick(context.Background(), "state", "", "", nil, 0)
			Expect(err).To(HaveOccurred())
		})

		It("errors on upstream failure and on a missing tier answer", func() {
			failing := newEngine(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})
			_, _, err := New("laya", failing.URL, "").Pick(context.Background(), "s", "", "", map[string]string{"a": "b"}, 0)
			Expect(err).To(HaveOccurred())

			empty := newEngine(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"answers":{}}`))
			})
			_, _, err = New("laya", empty.URL, "").Pick(context.Background(), "s", "", "", map[string]string{"a": "b"}, 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no tier choice"))
		})

		It("forwards min_confidence and treats an abstention as ErrAbstained", func() {
			var gotMinConfidence float64
			srv := newEngine(func(w http.ResponseWriter, r *http.Request) {
				var req predictRequest
				Expect(json.Unmarshal(received, &req)).To(Succeed())
				gotMinConfidence = req.MinConfidence
				_, _ = w.Write([]byte(`{"answers":{"tier":{"type":"choice","choice":"worker","confidence":0.3,"abstention":"abstained"}}}`))
			})
			_, _, err := New("laya", srv.URL, "").Pick(context.Background(), "s", "", "", map[string]string{"a": "b"}, 0.7)
			Expect(gotMinConfidence).To(BeNumerically("~", 0.7, 0.001))
			Expect(err).To(MatchError(ContainSubstring("abstained")))
			Expect(errors.Is(err, ErrAbstained)).To(BeTrue())
		})

		It("enforces the confidence floor locally on answer_confidence", func() {
			srv := newEngine(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"answers":{"tier":{"type":"choice","choice":"fast","confidence":0.9,"answer_confidence":0.2}}}`))
			})
			_, _, err := New("laya", srv.URL, "").Pick(context.Background(), "s", "", "", map[string]string{"a": "b"}, 0.5)
			Expect(errors.Is(err, ErrAbstained)).To(BeTrue())
		})
	})

	Describe("Set", func() {
		It("keeps configuration order and looks up by name", func() {
			laya := New("laya", "http://laya", "")
			jev := New("jev", "http://jev", "k", "jev-latest")
			s := NewSet(laya, jev)
			Expect(s.Len()).To(Equal(2))
			first, ok := s.First()
			Expect(ok).To(BeTrue())
			Expect(first.Name()).To(Equal("laya"))
			got, ok := s.Get("jev")
			Expect(ok).To(BeTrue())
			Expect(got.Models()).To(Equal([]string{"jev-latest"}))
			_, ok = s.Get("missing")
			Expect(ok).To(BeFalse())
		})
	})
})
