package auth

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewKeyStore", func() {
	When("loading from a file with keys and comments", func() {
		It("parses keys and ignores comments", func() {
			content := `# This is a comment
sk-key-one
sk-key-two

# Another comment
sk-key-three
`
			path := filepath.Join(GinkgoT().TempDir(), "keys.txt")
			Expect(os.WriteFile(path, []byte(content), 0644)).NotTo(HaveOccurred())

			ks, err := NewKeyStore(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(ks.Count()).To(Equal(3))
		})
	})

	When("INFERENCIA_API_KEYS is set", func() {
		It("uses env keys instead of file", func() {
			_ = os.Setenv("INFERENCIA_API_KEYS", "sk-env-one, sk-env-two")
			defer func() { _ = os.Unsetenv("INFERENCIA_API_KEYS") }()

			ks, err := NewKeyStore("")
			Expect(err).NotTo(HaveOccurred())
			Expect(ks.Count()).To(Equal(2))
		})
	})

	When("the keys file is empty (only comments)", func() {
		It("returns an error", func() {
			path := filepath.Join(GinkgoT().TempDir(), "empty.txt")
			Expect(os.WriteFile(path, []byte("# only comments\n"), 0644)).NotTo(HaveOccurred())

			_, err := NewKeyStore(path)
			Expect(err).To(HaveOccurred())
		})
	})

	When("the keys file does not exist", func() {
		It("returns an error", func() {
			_, err := NewKeyStore("/nonexistent/keys.txt")
			Expect(err).To(HaveOccurred())
		})
	})

	When("path is empty and INFERENCIA_API_KEYS is not set", func() {
		It("returns an error", func() {
			_ = os.Unsetenv("INFERENCIA_API_KEYS")
			_, err := NewKeyStore("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no keys file path"))
		})
	})

	When("INFERENCIA_API_KEYS is set but only commas or whitespace", func() {
		It("returns an error", func() {
			_ = os.Setenv("INFERENCIA_API_KEYS", "  , , ")
			defer func() { _ = os.Unsetenv("INFERENCIA_API_KEYS") }()
			_, err := NewKeyStore("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no valid keys"))
		})
	})
})

var _ = Describe("Validate", func() {
	BeforeEach(func() {
		_ = os.Setenv("INFERENCIA_API_KEYS", "sk-valid-key")
	})
	AfterEach(func() {
		_ = os.Unsetenv("INFERENCIA_API_KEYS")
	})

	When("the key is valid", func() {
		It("returns no error", func() {
			ks, err := NewKeyStore("")
			Expect(err).NotTo(HaveOccurred())
			Expect(ks.Validate("sk-valid-key")).NotTo(HaveOccurred())
		})
	})

	When("the key is invalid", func() {
		It("returns an error", func() {
			ks, err := NewKeyStore("")
			Expect(err).NotTo(HaveOccurred())
			Expect(ks.Validate("sk-wrong-key")).To(HaveOccurred())
		})
	})

	When("the key is empty", func() {
		It("returns an error", func() {
			ks, err := NewKeyStore("")
			Expect(err).NotTo(HaveOccurred())
			Expect(ks.Validate("")).To(HaveOccurred())
		})
	})
})
