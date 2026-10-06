package main

import (
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ASR transcription state", func() {
	It("replaces hypotheses, suppresses repeats, and returns correlated snapshots", func() {
		empty := sessionState{}
		Expect(empty.handleTranscription(transcriptionResult{err: errors.New("missing item ID")})).To(BeNil())
		Expect(empty.audio.failed).To(BeFalse())
		state := sessionState{audio: audioBuffer{itemID: "item_1", bytes: 2}}
		first := state.handleTranscription(transcriptionResult{audioItemID: "item_1", transcript: "hello"})
		Expect(first.EventID).NotTo(BeEmpty())
		contentIndex, transcript := 0, "hello"
		Expect(*first).To(Equal(serverEvent{EventID: first.EventID, Type: TranscriptionUpdated,
			ItemID: "item_1", ContentIndex: &contentIndex, Transcript: &transcript, Revision: 1}))
		Expect(state.handleTranscription(transcriptionResult{audioItemID: "item_1", transcript: "hello"})).To(BeNil())
		second := state.handleTranscription(transcriptionResult{audioItemID: "item_1", transcript: "goodbye"})
		Expect(*second.Transcript).To(Equal("goodbye"))
		Expect(second.Revision).To(Equal(2))
		Expect(second.EventID).NotTo(Equal(first.EventID))
		Expect(*first.Transcript).To(Equal("hello"))
		Expect(state.audio.transcript).To(Equal("goodbye"))
		for _, result := range []transcriptionResult{
			{audioItemID: "cleared", transcript: "stale"}, {audioItemID: "cleared", turnFinished: true},
			{audioItemID: "unknown", err: errors.New("stale failure")},
		} {
			Expect(state.handleTranscription(result)).To(BeNil())
		}
		Expect(state.audio).To(Equal(audioBuffer{itemID: "item_1", bytes: 2, transcript: "goodbye", revision: 2}))
	})

	It("assigns out-of-order pending results and retains history after unchanged finals", func() {
		state := sessionState{}
		for _, itemID := range []string{"item_1", "item_2"} {
			state.audio = audioBuffer{itemID: itemID, bytes: 2, transcript: "partial", revision: 1}
			commitAudio(clientEvent{}, &state)
		}
		Expect(state.awaitingTranscription).To(Equal(map[string]int{"item_1": 0, "item_2": 1}))
		for _, itemID := range []string{"item_2", "item_1"} {
			transcript := ""
			if itemID == "item_2" {
				transcript = "corrected"
			}
			update := state.handleTranscription(transcriptionResult{audioItemID: itemID, transcript: transcript})
			Expect(update.Type).To(Equal(TranscriptionUpdated))
			Expect(update.ItemID).To(Equal(itemID))
			final := state.handleTranscription(transcriptionResult{audioItemID: itemID, transcript: transcript, turnFinished: true})
			Expect(final.Type).To(Equal(TranscriptionCompleted))
			Expect(final.Revision).To(Equal(3))
			Expect(state.awaitingTranscription).NotTo(HaveKey(itemID))
			Expect(state.handleTranscription(transcriptionResult{audioItemID: itemID, transcript: "late", turnFinished: true})).To(BeNil())
			if itemID == "item_2" {
				Expect(state.awaitingTranscription).To(Equal(map[string]int{"item_1": 0}))
			} else {
				encoded, err := json.Marshal(final)
				Expect(err).NotTo(HaveOccurred())
				var fields map[string]json.RawMessage
				Expect(json.Unmarshal(encoded, &fields)).To(Succeed())
				Expect(string(fields["transcript"])).To(Equal(`""`))
				Expect(string(fields["content_index"])).To(Equal("0"))
			}
		}
		Expect(state.items).To(Equal([]conversationItem{
			{id: "item_1", role: "user", revision: 3},
			{id: "item_2", role: "user", transcript: "corrected", revision: 3},
		}))
		Expect(state.awaitingTranscription).To(BeEmpty())
		Expect(state.audio).To(Equal(audioBuffer{}))
	})

	It("emits failure once, keeps it pending until finish, and never replaces it with success", func() {
		state := sessionState{audio: audioBuffer{itemID: "item_1", bytes: 2, transcript: "partial", revision: 1}}
		failure := transcriptionResult{audioItemID: "item_1", err: errors.New("decoder failed")}
		event := state.handleTranscription(failure)
		Expect(event.Type).To(Equal(TranscriptionFailed))
		Expect(event.ItemID).To(Equal("item_1"))
		Expect(*event.ContentIndex).To(BeZero())
		Expect(*event.Error).To(Equal(eventError{Type: "server_error", Code: "transcription_failed", Message: "decoder failed"}))
		Expect(state.handleTranscription(failure)).To(BeNil())
		commitAudio(clientEvent{}, &state)
		Expect(state.handleTranscription(failure)).To(BeNil())
		Expect(state.handleTranscription(transcriptionResult{audioItemID: "item_1", transcript: "unexpected"})).To(BeNil())
		Expect(state.awaitingTranscription).To(Equal(map[string]int{"item_1": 0}))
		failure.turnFinished = true
		Expect(state.handleTranscription(failure)).To(BeNil())
		Expect(state.awaitingTranscription).To(BeEmpty())
		Expect(state.items[0]).To(Equal(conversationItem{id: "item_1", role: "user", transcript: "partial", revision: 1, failed: true}))
	})
})
