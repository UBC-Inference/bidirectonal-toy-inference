package main

import (
	"encoding/json"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Realtime ASR WebSocket lifecycle", func() {
	It("rejects unknown wire events without dispatching ASR operations", func() {
		var creations atomic.Int32
		connect, _ := realtimeTestServer(fakeASRRecognizer{newStream: func() (asrStream, error) {
			creations.Add(1)
			return newFakeASRStream(), nil
		}})
		connection := connect()
		sendRealtimeEvent(connection, clientEvent{EventID: "request_1", Type: "unknown", Audio: "AAA="})
		response := readRealtimeEvent(connection)
		Expect(response.Type).To(Equal(RealtimeError))
		Expect(response.Error.EventID).To(Equal("request_1"))
		sendRealtimeEvent(connection, clientEvent{Type: SessionUpdate, Session: json.RawMessage(`{}`)})
		Expect(readRealtimeEvent(connection).Type).To(Equal(SessionUpdated))
		Expect(creations.Load()).To(BeZero())
	})

	It("clears an in-flight turn, ignores its stale result, and starts a new turn", func() {
		first, second := newFakeASRStream(), newFakeASRStream()
		first.appendFn = func([]float32) (string, error) { return "stale", nil }
		connect, _ := realtimeTestServer(sequenceRecognizer(first, second))
		connection := connect()
		entered, release := blockFirstAppend(first)
		sendPCMAppend(connection, 0)
		Eventually(entered, asrTestTimeout).Should(BeClosed())
		sendRealtimeEvent(connection, clientEvent{Type: InputAudioBufferClear})
		cleared := readRealtimeEvent(connection)
		Expect(cleared.Type).To(Equal(InputAudioBufferCleared))
		Expect(cleared.ItemID).NotTo(BeEmpty())
		release()
		Eventually(first.closed, asrTestTimeout).Should(BeClosed())
		sendPCMAppend(connection, 0)
		update := readRealtimeEvent(connection)
		Expect(update.Type).To(Equal(TranscriptionUpdated))
		Expect(update.ItemID).NotTo(Equal(cleared.ItemID))
		Expect(*update.Transcript).To(Equal("partial"))
		sendRealtimeEvent(connection, clientEvent{Type: InputAudioBufferCommit})
		commit := readRealtimeEvent(connection)
		Expect(commit.Type).To(Equal(InputAudioBufferCommitted))
		Expect(commit.PreviousItemID).To(BeNil())
		Expect(readRealtimeEvent(connection).Type).To(Equal(TranscriptionCompleted))
		expectClosedStream(first, "append", "close")
	})

	It("keeps sessions independent and closes each active stream on disconnect", func() {
		firstStream, secondStream, active := newFakeASRStream(), newFakeASRStream(), newFakeASRStream()
		connect, finished := realtimeTestServer(sequenceRecognizer(firstStream, secondStream, active))
		first, second := connect(), connect()
		sendRealtimeEvent(first, clientEvent{Type: SessionUpdate, Session: json.RawMessage(`{"instructions":"first only"}`)})
		firstSession := readRealtimeEvent(first)
		Expect(firstSession.Session.Instructions).To(Equal("first only"))
		sendRealtimeEvent(second, clientEvent{Type: SessionUpdate, Session: json.RawMessage(`{}`)})
		secondSession := readRealtimeEvent(second)
		Expect(secondSession.Session.Instructions).To(BeEmpty())
		Expect(firstSession.Session.ID).NotTo(Equal(secondSession.Session.ID))
		sendPCMAppend(first, 0)
		Expect(readRealtimeEvent(first).Type).To(Equal(TranscriptionUpdated))
		sendPCMAppend(second, 0)
		update := readRealtimeEvent(second)
		Expect(update.Type).To(Equal(TranscriptionUpdated))
		Expect(*update.Transcript).To(Equal("partial"))
		Expect(first.Close()).To(Succeed())
		Eventually(finished, asrTestTimeout).Should(Receive())
		Expect(firstStream.closed).To(BeClosed())
		Expect(secondStream.closed).NotTo(BeClosed())
		sendRealtimeEvent(second, clientEvent{Type: InputAudioBufferCommit})
		commit := readRealtimeEvent(second)
		Expect(commit.Type).To(Equal(InputAudioBufferCommitted))
		Expect(commit.ItemID).To(Equal(update.ItemID))
		Expect(commit.PreviousItemID).To(BeNil())
		final := readRealtimeEvent(second)
		Expect(final.Type).To(Equal(TranscriptionCompleted))
		Expect(final.ItemID).To(Equal(commit.ItemID))
		Expect(*final.Transcript).To(Equal("final"))
		Expect(final.Revision).To(Equal(2))
		expectClosedStream(secondStream, "append", "finish", "close")
		sendPCMAppend(second, 0)
		Expect(readRealtimeEvent(second).Type).To(Equal(TranscriptionUpdated))
		Expect(second.Close()).To(Succeed())
		Eventually(finished, asrTestTimeout).Should(Receive())
		expectClosedStream(active, "append", "close")
	})
})
