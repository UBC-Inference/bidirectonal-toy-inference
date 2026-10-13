package server

import (
	"testing"
	"time"

	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/asr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const asrTestTimeout = 3 * time.Second

type asrStream = asr.Stream

type fakeASRRecognizer struct{ newStream func() (asr.Stream, error) }

func (recognizer fakeASRRecognizer) NewStream() (asr.Stream, error) {
	return recognizer.newStream()
}

type fakeASRStream struct{}

func newFakeASRStream() *fakeASRStream                         { return &fakeASRStream{} }
func (stream *fakeASRStream) Append([]float32) (string, error) { return "", nil }
func (stream *fakeASRStream) Finish() (string, error)          { return "", nil }
func (stream *fakeASRStream) Close()                           {}

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Suite")
}
