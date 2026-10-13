package asr

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestASR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ASR Suite")
}
