package realtime

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PCM conversion", func() {
	It("decodes signed little-endian PCM16 edges", func() {
		samples, err := pcm16ToFloat([]byte{0, 128, 255, 127, 255, 255, 0, 0, 1, 0, 0, 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(samples).To(Equal([]float32{-1, 32767.0 / 32768, -1.0 / 32768, 0, 1.0 / 32768, 256.0 / 32768}))
		samples, err = pcm16ToFloat(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(samples).To(BeEmpty())
		samples, err = pcm16ToFloat([]byte{0})
		Expect(err).To(HaveOccurred())
		Expect(samples).To(BeNil())
	})
})
