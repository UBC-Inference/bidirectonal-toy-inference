package main

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Realtime ASR backpressure", func() {
	It("preserves FIFO audio and both turns when the command queue is saturated", func() {
		const appendCount = 40
		first, second := newFakeASRStream(), newFakeASRStream()
		first.appendFn = func(samples []float32) (string, error) { return fmt.Sprintf("sample_%d", int(samples[0]*32768)), nil }
		connect, finished := realtimeTestServer(sequenceRecognizer(first, second))
		connection := connect()
		entered, release := blockFirstAppend(first)
		sendPCMAppend(connection, 1)
		Eventually(entered, asrTestTimeout).Should(BeClosed())
		for value := 2; value <= 17; value++ {
			sendPCMAppend(connection, value)
		}
		// This barrier proves all 16 queued commands were accepted while Append is blocked.
		sendRealtimeEvent(connection, clientEvent{Type: SessionUpdate, Session: json.RawMessage(`{"instructions":"queue full"}`)})
		barrier := readRealtimeEvent(connection)
		Expect(barrier.Type).To(Equal(SessionUpdated))
		Expect(barrier.Session.Instructions).To(Equal("queue full"))
		for value := 18; value <= appendCount; value++ {
			sendPCMAppend(connection, value)
		}
		sendRealtimeEvent(connection, clientEvent{Type: InputAudioBufferCommit})
		sendPCMAppend(connection, 100)
		sendRealtimeEvent(connection, clientEvent{Type: InputAudioBufferCommit})
		sendRealtimeEvent(connection, clientEvent{Type: SessionUpdate, Session: json.RawMessage(`{"instructions":"pipeline consumed"}`)})
		Expect(first.callSnapshot()).To(Equal([]string{"append"}))
		release()

		updates, committed := map[string]int{}, map[string]bool{}
		var finals []string
		barriers := 0
		for eventIndex := 0; eventIndex < appendCount+6; eventIndex++ {
			event := readRealtimeEvent(connection)
			Expect(event.Error).To(BeNil())
			switch event.Type {
			case TranscriptionUpdated:
				Expect(event.ItemID).To(BeElementOf("item_1", "item_2"))
				updates[event.ItemID]++
				Expect(event.Revision).To(Equal(updates[event.ItemID]))
				if event.ItemID == "item_1" {
					Expect(*event.Transcript).To(Equal(fmt.Sprintf("sample_%d", updates[event.ItemID])))
				} else {
					Expect(*event.Transcript).To(Equal("partial"))
				}
			case InputAudioBufferCommitted:
				Expect(event.ItemID).To(BeElementOf("item_1", "item_2"))
				Expect(committed[event.ItemID]).To(BeFalse())
				if event.ItemID == "item_1" {
					Expect(event.PreviousItemID).To(BeNil())
				} else {
					Expect(committed["item_1"]).To(BeTrue())
					Expect(event.PreviousItemID).To(HaveValue(Equal("item_1")))
				}
				committed[event.ItemID] = true
			case TranscriptionCompleted:
				Expect(committed[event.ItemID]).To(BeTrue())
				Expect(event.Transcript).To(HaveValue(Equal("final")))
				Expect(event.Revision).To(Equal(updates[event.ItemID] + 1))
				finals = append(finals, event.ItemID)
			case SessionUpdated:
				Expect(event.Session.Instructions).To(Equal("pipeline consumed"))
				barriers++
			default:
				Fail(fmt.Sprintf("unexpected event: %+v", event))
			}
		}
		Expect(updates).To(Equal(map[string]int{"item_1": appendCount, "item_2": 1}))
		Expect(finals).To(Equal([]string{"item_1", "item_2"}))
		Expect(barriers).To(Equal(1))
		for turn, stream := range []*fakeASRStream{first, second} {
			Eventually(stream.closed, asrTestTimeout).Should(BeClosed())
			expectedSamples := [][]float32{{100.0 / 32768}}
			if turn == 0 {
				expectedSamples = nil
				for value := 1; value <= appendCount; value++ {
					expectedSamples = append(expectedSamples, []float32{float32(value) / 32768})
				}
			}
			expectedCalls := make([]string, len(expectedSamples))
			for index := range expectedCalls {
				expectedCalls[index] = "append"
			}
			expectedCalls = append(expectedCalls, "finish", "close")
			Expect(stream.samples).To(Equal(expectedSamples))
			Expect(stream.callSnapshot()).To(Equal(expectedCalls))
			Expect(stream.concurrent.Load()).To(BeFalse())
		}
		Expect(connection.Close()).To(Succeed())
		Eventually(finished, asrTestTimeout).Should(Receive())
	})
})
