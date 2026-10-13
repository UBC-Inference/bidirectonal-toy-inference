package asr

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Native ASR startup path validation", func() {
	paths := []string{"encoder", "decoder", "joiner", "tokens"}
	var directory, readable string
	var config Config
	BeforeEach(func() {
		directory = GinkgoT().TempDir()
		readable = filepath.Join(directory, "readable")
		Expect(os.WriteFile(readable, []byte("validation fixture, not a model"), 0600)).To(Succeed())
		config = Config{EncoderPath: readable, DecoderPath: readable, JoinerPath: readable, TokensPath: readable}
	})

	for index, name := range paths {
		DescribeTable(name+" rejects invalid paths before native initialization", func(kind string) {
			var path, message string
			switch kind {
			case "missing":
				message = name + " path is required"
			case "nonexistent":
				path = filepath.Join(directory, "nonexistent")
				message = "open " + name + ":"
			case "directory":
				path = directory
				message = name + " must be a nonempty regular file"
			case "empty":
				path = filepath.Join(directory, "empty")
				Expect(os.WriteFile(path, nil, 0600)).To(Succeed())
				message = name + " must be a nonempty regular file"
			}
			configPaths := []*string{&config.EncoderPath, &config.DecoderPath, &config.JoinerPath, &config.TokensPath}
			*configPaths[index] = path
			recognizer, err := NewNativeRecognizer(config)
			Expect(recognizer).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(message))
		}, Entry("missing", "missing"), Entry("nonexistent", "nonexistent"), Entry("directory", "directory"), Entry("empty", "empty"))
	}

	It("accepts readable nonempty paths and reaches the next missing path", func() {
		// A missing final path prevents the readable dummy files from reaching native code.
		config.TokensPath = ""
		recognizer, err := NewNativeRecognizer(config)
		Expect(recognizer).To(BeNil())
		Expect(err).To(MatchError("tokens path is required"))
	})
})
