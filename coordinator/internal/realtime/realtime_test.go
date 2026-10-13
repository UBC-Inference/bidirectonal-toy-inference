package realtime

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCoordinator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Coordinator Suite")
}

var _ = Describe("Coordinator", func() {
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
			Expect(response.Type).To(Equal(RealtimeError))
			Expect(response.Error.EventID).To(Equal("request_1"))
			Expect(config.Instructions).To(Equal("before"))
		})
	})

	Describe("audio buffering", func() {
		It("links committed turns and retains only transcript history", func() {
			state := sessionState{audio: audioBuffer{itemID: "item_1", bytes: 8, transcript: "hello", revision: 2}}
			first := commitAudio(clientEvent{}, &state)
			Expect(first.Type).To(Equal(InputAudioBufferCommitted))
			Expect(first.EventID).NotTo(BeEmpty())
			Expect(first.ItemID).To(Equal("item_1"))
			Expect(first.PreviousItemID).To(BeNil())
			Expect(state.audio).To(Equal(audioBuffer{}))
			firstItem := conversationItem{id: "item_1", role: "user", transcript: "hello", revision: 2}
			Expect(state.items).To(Equal([]conversationItem{firstItem}))
			state.audio = audioBuffer{itemID: "item_2", bytes: 2, failed: true}
			second := commitAudio(clientEvent{}, &state)
			Expect(*second.PreviousItemID).To(Equal(first.ItemID))
			Expect(second.ItemID).To(Equal("item_2"))
			Expect(second.EventID).NotTo(Equal(first.EventID))
			Expect(state.items).To(Equal([]conversationItem{firstItem, {id: "item_2", role: "user", failed: true}}))
			Expect(state.awaitingTranscription).To(Equal(map[string]int{"item_1": 0, "item_2": 1}))
			Expect(state.audio).To(Equal(audioBuffer{}))
		})

		It("allocates IDs only for valid appends and does not reuse a cleared ID", func() {
			state := sessionState{}
			command, response := state.handleClientEvent(clientEvent{Type: InputAudioBufferAppend, Audio: "!"})
			Expect(command).To(BeNil())
			Expect(response.Type).To(Equal(RealtimeError))
			Expect(state.nextItemID).To(BeZero())
			command, response = state.handleClientEvent(clientEvent{Type: InputAudioBufferAppend, Audio: "AAA="})
			Expect(response).To(BeNil())
			Expect(command.audioItemID).To(Equal("item_1"))
			command, response = state.handleClientEvent(clientEvent{Type: InputAudioBufferClear})
			Expect(command.kind).To(Equal(asrDiscardTurn))
			Expect(response.ItemID).To(Equal("item_1"))
			Expect(state.audio).To(Equal(audioBuffer{}))
			command, response = state.handleClientEvent(clientEvent{Type: InputAudioBufferAppend, Audio: "AAA="})
			Expect(response).To(BeNil())
			Expect(command.audioItemID).To(Equal("item_2"))
			Expect(state.nextItemID).To(Equal(2))
			command, response = state.handleClientEvent(clientEvent{Type: InputAudioBufferCommit})
			Expect(command.kind).To(Equal(asrFinishTurn))
			Expect(response.ItemID).To(Equal("item_2"))
			Expect(state.awaitingTranscription).To(Equal(map[string]int{"item_2": 0}))
		})

		DescribeTable("rejects invalid input without modifying the byte count", func(audio, code string) {
			buffer := audioBuffer{itemID: "item_1", bytes: 2}
			samples, response := decodeAudio(clientEvent{EventID: "request_1", Audio: audio}, buffer.bytes)
			Expect(samples).To(BeNil())
			Expect(response.Type).To(Equal(RealtimeError))
			Expect(response.Error.Code).To(Equal(code))
			Expect(response.Error.EventID).To(Equal("request_1"))
			Expect(response.Error.Param).To(Equal("audio"))
			Expect(buffer).To(Equal(audioBuffer{itemID: "item_1", bytes: 2}))
		},
			Entry("missing audio", "", "missing_required_parameter"),
			Entry("malformed base64", "!", "invalid_value"),
			Entry("base64 without audio samples", "\n", "invalid_value"),
			Entry("incomplete PCM16 sample", "AA==", "invalid_value"),
		)

		It("enforces the per-turn audio limit", func() {
			buffer := audioBuffer{bytes: maxBufferedAudioSize}
			samples, response := decodeAudio(clientEvent{Audio: "AAA="}, buffer.bytes)
			Expect(samples).To(BeNil())
			Expect(response.Error.Code).To(Equal("buffer_too_large"))
			Expect(buffer.bytes).To(Equal(maxBufferedAudioSize))
			samples, response = decodeAudio(clientEvent{Audio: base64.StdEncoding.EncodeToString([]byte{0, 128})}, maxBufferedAudioSize-2)
			Expect(response).To(BeNil())
			Expect(samples).To(Equal([]float32{-1}))
		})

		It("rejects empty commits without advancing item history", func() {
			state := sessionState{items: []conversationItem{{id: "existing", role: "user"}}}
			response := commitAudio(clientEvent{}, &state)
			Expect(response.Error.Code).To(Equal("input_audio_buffer_commit_empty"))
			Expect(state.items).To(Equal([]conversationItem{{id: "existing", role: "user"}}))
		})
	})
})
