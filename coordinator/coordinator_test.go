package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCoordinator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Coordinator Suite")
}

var _ = Describe("Coordinator", func() {
	DescribeTable("bearer authentication middleware", func(header string, status int) {
		router := gin.New()
		router.GET("/protected", requireAuth("secret"), func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		Expect(response.Code).To(Equal(status))
		if status == http.StatusUnauthorized {
			Expect(response.Header().Get("WWW-Authenticate")).To(Equal("Bearer"))
		}
	},
		Entry("accepts the configured token", "Bearer secret", http.StatusNoContent),
		Entry("rejects missing credentials", "", http.StatusUnauthorized),
		Entry("rejects an incorrect token", "Bearer wrong", http.StatusUnauthorized),
	)

	Describe("session updates", func() {
		It("applies instructions and returns a snapshot", func() {
			config := sessionConfig{Instructions: "before"}
			response := updateSession(clientEvent{
				Session: json.RawMessage(`{"instructions":"after"}`),
			}, &config)
			Expect(response.Type).To(Equal(SessionUpdated))
			Expect(config.Instructions).To(Equal("after"))
			config.Instructions = "later"
			Expect(response.Session.Instructions).To(Equal("after"))
		})

		It("rejects an unsupported sample rate without modifying configuration", func() {
			config := sessionConfig{Instructions: "before"}
			response := updateSession(clientEvent{
				EventID: "request_1",
				Session: json.RawMessage(`{"instructions":"after","audio":{"input":{"format":{"rate":16000}}}}`),
			}, &config)
			Expect(response.Type).To(Equal(Error))
			Expect(response.Error.EventID).To(Equal("request_1"))
			Expect(config.Instructions).To(Equal("before"))
		})
	})

	Describe("audio buffering", func() {
		It("retains appended PCM and links committed turns", func() {
			var buffer audioBuffer
			var previousItemID *string
			audio := []byte{0, 0, 1, 0}
			event := clientEvent{Audio: base64.StdEncoding.EncodeToString(audio)}
			Expect(appendAudio(event, &buffer)).To(BeNil())
			Expect(appendAudio(event, &buffer)).To(BeNil())
			first := commitAudio(clientEvent{}, &buffer, &previousItemID)
			Expect(first.Type).To(Equal(InputAudioBufferCommitted))
			Expect(first.PreviousItemID).To(BeNil())
			Expect(buffer.pending).To(BeEmpty())
			Expect(buffer.committed).To(Equal(append(append([]byte{}, audio...), audio...)))
			Expect(appendAudio(event, &buffer)).To(BeNil())
			second := commitAudio(clientEvent{}, &buffer, &previousItemID)
			Expect(*second.PreviousItemID).To(Equal(first.ItemID))
			Expect(buffer.committed).To(Equal(audio))
		})

		DescribeTable("rejects invalid input without modifying buffered audio", func(audio string) {
			buffer := audioBuffer{pending: []byte{0, 0}}
			response := appendAudio(clientEvent{Audio: audio}, &buffer)
			Expect(response.Type).To(Equal(Error))
			Expect(buffer.pending).To(Equal([]byte{0, 0}))
		},
			Entry("missing audio", ""),
			Entry("malformed base64", "!"),
			Entry("incomplete PCM16 sample", "AA=="),
		)

		It("enforces the per-turn audio limit", func() {
			buffer := audioBuffer{pending: make([]byte, maxBufferedAudioSize)}
			response := appendAudio(clientEvent{Audio: "AAA="}, &buffer)
			Expect(response.Error.Code).To(Equal("buffer_too_large"))
			Expect(buffer.pending).To(HaveLen(maxBufferedAudioSize))
		})

		It("rejects empty commits without advancing item history", func() {
			var buffer audioBuffer
			var previousItemID *string
			response := commitAudio(clientEvent{}, &buffer, &previousItemID)
			Expect(response.Error.Code).To(Equal("input_audio_buffer_commit_empty"))
			Expect(previousItemID).To(BeNil())
		})
	})
})
