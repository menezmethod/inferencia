package ledger

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func partial(err error) *PartialError {
	var pe *PartialError
	Expect(errors.As(err, &pe)).To(BeTrue(), "want *PartialError, got %v", err)
	return pe
}

var _ = Describe("Window", func() {
	It("treats zero bounds as open", func() {
		Expect(Window{}.Contains(ts("2001-01-01T00:00:00Z"))).To(BeTrue())
	})
	It("includes both bounds", func() {
		w := Window{Since: ts("2026-01-01T00:00:00Z"), Until: ts("2026-01-02T00:00:00Z")}
		Expect(w.Contains(w.Since)).To(BeTrue())
		Expect(w.Contains(w.Until)).To(BeTrue())
		Expect(w.Contains(w.Until.Add(time.Nanosecond))).To(BeFalse())
		Expect(w.Contains(w.Since.Add(-time.Nanosecond))).To(BeFalse())
	})
})

var _ = Describe("ClaudeCode", func() {
	w := Window{Until: ts("2026-09-30T23:59:59Z")}

	It("deduplicates by message id across files, keeping the copy with the most output", func() {
		us, err := ClaudeCode("testdata/claude", w)
		Expect(us).To(HaveLen(4)) // msg_A, msg_B, msg_D, msg_E; msg_C is after Until
		t := Sum(us)
		Expect(t.Input).To(Equal(int64(17)))
		Expect(t.CacheRead).To(Equal(int64(1640)))
		Expect(t.CacheWrite).To(Equal(int64(100)))
		Expect(t.Output).To(Equal(int64(13))) // msg_A: 7 not 5; msg_B: 3, not the resumed copy's 1
		// msg_X has "input_tokens":1.5: counted as unreadable, never silently dropped.
		Expect(partial(err).BadLines).To(Equal(1))
	})

	It("tags sidechain lines and files under a subagents directory as delegated", func() {
		us, _ := ClaudeCode("testdata/claude", w)
		delegated := map[string]bool{}
		for _, u := range us {
			Expect(u.Harness).To(Equal("claude-code"))
			Expect(u.Upstream).To(Equal("anthropic"))
			delegated[u.Model+"/"+u.Time.Format(time.RFC3339)] = u.Delegated
		}
		Expect(delegated).To(HaveKeyWithValue("claude-sonnet-test/2026-09-12T00:00:00Z", true)) // isSidechain
		Expect(delegated).To(HaveKeyWithValue("claude-sonnet-test/2026-09-12T00:00:10Z", true)) // subagents/ path only
		Expect(delegated).To(HaveKeyWithValue("claude-opus-test/2026-09-11T00:00:00Z", false))
	})

	It("compares Since by instant across time zones", func() {
		us, _ := ClaudeCode("testdata/claude", Window{Since: ts("2026-09-11T00:00:00+02:00"), Until: w.Until})
		Expect(us).To(HaveLen(3)) // msg_A (09-10T00:00Z) falls before 09-10T22:00Z
	})

	It("follows a symlinked root", func() {
		link := filepath.Join(GinkgoT().TempDir(), "projects")
		abs, err := filepath.Abs("testdata/claude")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Symlink(abs, link)).To(Succeed())
		us, _ := ClaudeCode(link, w)
		Expect(us).To(HaveLen(4))
	})

	It("does not mark calls delegated because the root path contains 'subagents'", func() {
		root := filepath.Join(GinkgoT().TempDir(), "subagents", "projects")
		Expect(os.MkdirAll(root, 0o755)).To(Succeed())
		line := `{"type":"assistant","sessionId":"s","timestamp":"2026-09-01T00:00:00Z","message":{"id":"m","model":"x","usage":{"input_tokens":1}}}` + "\n"
		Expect(os.WriteFile(filepath.Join(root, "s.jsonl"), []byte(line), 0o644)).To(Succeed())
		us, err := ClaudeCode(root, Window{})
		Expect(err).NotTo(HaveOccurred())
		Expect(us).To(HaveLen(1))
		Expect(us[0].Delegated).To(BeFalse())
	})

	It("returns ErrNotExist for a missing root", func() {
		_, err := ClaudeCode("testdata/does-not-exist", w)
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	})
})

var _ = Describe("Codex", func() {
	It("uses the last running total at or before Until and splits cached from uncached input", func() {
		us, err := Codex("testdata/codex", Window{Until: ts("2026-10-01T23:59:59Z")})
		Expect(err).NotTo(HaveOccurred())
		Expect(us).To(HaveLen(1)) // file b has no token events
		u := us[0]
		Expect(u.Model).To(Equal("gpt-test"))
		Expect(u.Upstream).To(Equal("openai"))
		Expect(u.Billing).To(Equal(BillingPlan))
		Expect(u.Delegated).To(BeTrue())
		Expect(u.Calls).To(Equal(2))
		Expect(u.Input).To(Equal(int64(200)))
		Expect(u.CacheRead).To(Equal(int64(2800)))
		Expect(u.Output).To(Equal(int64(120)))
		Expect(u.Reasoning).To(Equal(int64(70)))
	})

	It("subtracts the total reached before Since and keeps the model set before the window", func() {
		us, err := Codex("testdata/codex", Window{Since: ts("2026-10-01T01:30:00Z"), Until: ts("2026-10-02T03:00:00Z")})
		Expect(err).NotTo(HaveOccurred())
		Expect(us).To(HaveLen(1))
		u := us[0]
		Expect(u.Model).To(Equal("gpt-test")) // turn_context is before Since
		Expect(u.Calls).To(Equal(2))
		Expect(u.Input).To(Equal(int64(200)))      // (5000-1000) - (4700-900)
		Expect(u.CacheRead).To(Equal(int64(3800))) // 4700-900
		Expect(u.Output).To(Equal(int64(150)))
	})

	It("adds across a counter reset and ignores events from before the session started", func() {
		us, err := Codex("testdata/codex-reset", Window{})
		Expect(err).NotTo(HaveOccurred())
		Expect(us).To(HaveLen(1))
		u := us[0]
		Expect(u.Delegated).To(BeFalse())
		Expect(u.Model).To(Equal("gpt-reset"))
		Expect(u.Calls).To(Equal(2))              // the replayed 99,999 event is not ours
		Expect(u.CacheRead).To(Equal(int64(950))) // 900 + 50
		Expect(u.Input).To(Equal(int64(150)))     // (1000-900) + (100-50)
		Expect(u.Output).To(Equal(int64(45)))     // 40 + 5, not 5
	})

	It("returns nothing when the window holds no token events", func() {
		us, err := Codex("testdata/codex", Window{Until: ts("2026-09-01T00:00:00Z")})
		Expect(err).NotTo(HaveOccurred())
		Expect(us).To(BeEmpty())
	})
})

var _ = Describe("CommandCode", func() {
	It("reads usage lines, skips checkpoint files, clamps and keeps harness cost", func() {
		us, err := CommandCode("testdata/commandcode", Window{Until: ts("2026-10-06T00:10:34Z")})
		Expect(err).NotTo(HaveOccurred()) // "{broken" has no "usage" field, so it is not a usage line
		Expect(us).To(HaveLen(2))
		by := map[string]Usage{}
		for _, u := range us {
			by[u.Model] = u
		}
		d := by["deepseek/test"]
		Expect(d.Input).To(Equal(int64(100))) // inputTokens includes cache reads
		Expect(d.CacheRead).To(Equal(int64(900)))
		Expect(d.HasCost).To(BeTrue())
		Expect(d.CostUSD).To(BeNumerically("~", 0.01, 1e-9))
		Expect(d.Session).To(Equal("s1"))
		odd := by["odd/test"]
		Expect(odd.Input).To(Equal(int64(0))) // 10 - 40 clamps to 0
		Expect(odd.HasCost).To(BeFalse())
	})
})

var _ = Describe("OpenCode", func() {
	var db string

	BeforeEach(func() {
		if _, err := exec.LookPath("sqlite3"); err != nil {
			Skip("sqlite3 not on PATH")
		}
		db = filepath.Join(GinkgoT().TempDir(), "opencode.db")
		schema := `
CREATE TABLE session(id TEXT PRIMARY KEY, parent_id TEXT);
CREATE TABLE message(id TEXT PRIMARY KEY, session_id TEXT NOT NULL, data TEXT NOT NULL);
INSERT INTO session VALUES ('s1', NULL), ('s2', 's1');
INSERT INTO message VALUES
 ('m1','s1','{"role":"assistant","providerID":"opencode-go","modelID":"glm-test","time":{"created":1789000000000},"cost":0.02,"tokens":{"input":100,"output":10,"reasoning":5,"cache":{"read":900,"write":0}}}'),
 ('m2','s2','{"role":"assistant","providerID":"openrouter","modelID":"z-ai/glm-test","time":{"created":1789000001000},"cost":0.01,"tokens":{"input":50,"output":5,"reasoning":0,"cache":{"read":450,"write":0}}}'),
 ('m3','s1','{"role":"user","time":{"created":1789000001500}}'),
 ('m4','s1','{"role":"assistant","providerID":"opencode-go","modelID":"glm-test","time":{"created":1795000000000},"cost":1,"tokens":{"input":1,"output":1,"reasoning":0,"cache":{"read":0,"write":0}}}'),
 ('m5','s1','{"role":"assistant","providerID":"ollama","modelID":"local-test","time":{"created":1789000002000},"cost":0,"tokens":{"input":10,"output":1,"reasoning":0,"cache":{"read":0,"write":0}}}'),
 ('m6','s1','not json at all'),
 ('m7','s1','{"role":"assistant","providerID":"openrouter","modelID":"no-time","tokens":{"input":5}}');`
		cmd := exec.Command("sqlite3", db)
		cmd.Stdin = strings.NewReader(schema)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
	})

	It("reads assistant messages in the window, survives a corrupt row and tags subagents and billing", func() {
		us, err := OpenCode(db, Window{Until: time.UnixMilli(1790000000000)})
		Expect(partial(err).BadLines).To(Equal(1)) // m7 has no creation time; m6 is filtered by json_valid
		Expect(us).To(HaveLen(3))
		by := map[string]Usage{}
		for _, u := range us {
			by[u.Model] = u
		}
		Expect(by["glm-test"].Billing).To(Equal(BillingPlan))
		Expect(by["glm-test"].Output).To(Equal(int64(15))) // output + reasoning
		Expect(by["glm-test"].Delegated).To(BeFalse())
		Expect(by["z-ai/glm-test"].Billing).To(Equal(BillingAPI))
		Expect(by["z-ai/glm-test"].Delegated).To(BeTrue())
		Expect(by["local-test"].Billing).To(Equal(BillingLocal))
		Expect(by["local-test"].HasCost).To(BeTrue())
		t := Sum(us)
		Expect(t.CostUSD).To(BeNumerically("~", 0.03, 1e-9))
		Expect(t.CacheShare()).To(BeNumerically("~", 1350.0/(160.0+1350.0), 1e-9))
	})

	It("applies Since", func() {
		us, _ := OpenCode(db, Window{Since: time.UnixMilli(1789000001000), Until: time.UnixMilli(1790000000000)})
		Expect(us).To(HaveLen(2)) // m2 and m5
	})

	It("fails with ErrNotExist when the database does not exist", func() {
		_, err := OpenCode(filepath.Join(os.TempDir(), "no-such-opencode.db"), Window{})
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	})
})

var _ = Describe("ClaudeCode cache writes", func() {
	It("records the 1-hour part of cache writes", func() {
		root := GinkgoT().TempDir()
		line := `{"type":"assistant","sessionId":"s","timestamp":"2026-09-01T00:00:00Z","message":{"id":"m","model":"x","usage":{"input_tokens":1,"cache_creation_input_tokens":300,"cache_creation":{"ephemeral_1h_input_tokens":200,"ephemeral_5m_input_tokens":100}}}}` + "\n"
		Expect(os.WriteFile(filepath.Join(root, "s.jsonl"), []byte(line), 0o644)).To(Succeed())
		us, err := ClaudeCode(root, Window{})
		Expect(err).NotTo(HaveOccurred())
		Expect(us[0].CacheWrite).To(Equal(int64(300)))
		Expect(us[0].CacheWrite1h).To(Equal(int64(200)))
	})
})
