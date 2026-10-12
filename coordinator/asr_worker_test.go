package main

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ASR worker", func() {
	It("rejects invalid commands and switching IDs without disturbing the original turn", func() {
		stream := newFakeASRStream()
		commands, results, _, _ := startTestWorker(sequenceRecognizer(stream, newFakeASRStream()), 16)
		Expect(exchangeASRCommand(commands, results, asrCommand{kind: asrAppendAudio}).err).To(HaveOccurred())
		exchangeASRCommand(commands, results, asrCommand{kind: asrAppendAudio, audioItemID: "original"})
		for _, command := range []asrCommand{
			{kind: asrAppendAudio, audioItemID: "other"}, {kind: asrFinishTurn, audioItemID: "other"},
			{kind: asrAppendAudio}, {kind: asrFinishTurn}, {kind: asrCommandType(-1), audioItemID: "original"},
		} {
			result := exchangeASRCommand(commands, results, command)
			Expect(result.audioItemID).To(Equal(command.audioItemID))
			Expect(result.err).To(HaveOccurred())
			Expect(stream.closed).NotTo(BeClosed())
			Expect(stream.callSnapshot()).To(Equal([]string{"append"}))
		}
		Expect(exchangeASRCommand(commands, results, asrCommand{kind: asrAppendAudio, audioItemID: "original", samples: []float32{-1, 0, 0.5}})).To(Equal(transcriptionResult{audioItemID: "original", transcript: "partial"}))
		Expect(exchangeASRCommand(commands, results, asrCommand{kind: asrFinishTurn, audioItemID: "original"})).To(Equal(transcriptionResult{audioItemID: "original", transcript: "final", turnFinished: true}))
		expectClosedStream(stream, "append", "append", "finish", "close")
		Expect(stream.samples[1]).To(Equal([]float32{-1, 0, 0.5}))
		Expect(exchangeASRCommand(commands, results, asrCommand{kind: asrAppendAudio, audioItemID: "next"})).To(Equal(transcriptionResult{audioItemID: "next", transcript: "partial"}))
	})

	DescribeTable("propagates failures, closes streams, and recovers", func(stage string, discard bool) {
		broken, healthy := newFakeASRStream(), newFakeASRStream()
		failure := errors.New(stage + " failed")
		if stage == "append" {
			broken.appendFn = func([]float32) (string, error) { return "", failure }
		}
		if stage == "finish" {
			broken.finishFn = func() (string, error) { return "", failure }
		}
		creations := 0
		commands, results, _, _ := startTestWorker(fakeASRRecognizer{newStream: func() (asrStream, error) {
			creations++
			if creations > 1 {
				return healthy, nil
			}
			if stage == "creation" {
				return nil, failure
			}
			return broken, nil
		}}, 16)
		result := exchangeASRCommand(commands, results, asrCommand{kind: asrAppendAudio, audioItemID: "bad"})
		Expect(result.audioItemID).To(Equal("bad"))
		if stage == "finish" {
			Expect(result.err).NotTo(HaveOccurred())
		} else {
			Expect(result.err).To(MatchError(failure))
			Expect(exchangeASRCommand(commands, results, asrCommand{kind: asrAppendAudio, audioItemID: "bad"}).err).To(MatchError(failure))
		}
		if discard {
			commands <- asrCommand{kind: asrDiscardTurn}
		} else {
			Expect(exchangeASRCommand(commands, results, asrCommand{kind: asrFinishTurn, audioItemID: "bad"})).To(Equal(transcriptionResult{audioItemID: "bad", turnFinished: true, err: failure}))
		}
		if stage != "creation" {
			calls := []string{"append", "close"}
			if stage == "finish" {
				calls = []string{"append", "finish", "close"}
			}
			expectClosedStream(broken, calls...)
		}
		Expect(exchangeASRCommand(commands, results, asrCommand{kind: asrAppendAudio, audioItemID: "good"})).To(Equal(transcriptionResult{audioItemID: "good", transcript: "partial"}))
		Expect(exchangeASRCommand(commands, results, asrCommand{kind: asrFinishTurn, audioItemID: "good"}).err).NotTo(HaveOccurred())
		Expect(creations).To(Equal(2))
	}, Entry("creation", "creation", false), Entry("append", "append", false), Entry("finish", "finish", false), Entry("clear failed turn", "creation", true))

	DescribeTable("closes discarded or cancelled streams without finishing", func(termination string) {
		stream := newFakeASRStream()
		capacity := 16
		if termination == "undelivered result" {
			capacity = 0
		}
		commands, results, done, cancel := startTestWorker(sequenceRecognizer(stream), capacity)
		entered, release := blockFirstAppend(stream)
		commands <- asrCommand{kind: asrAppendAudio, audioItemID: "item_1"}
		Eventually(entered, asrTestTimeout).Should(BeClosed())
		if termination != "active append" {
			release()
			if capacity > 0 {
				receiveTranscription(results)
			}
		}
		switch termination {
		case "clear":
			commands <- asrCommand{kind: asrDiscardTurn}
		case "channel close":
			close(commands)
		default:
			cancel()
		}
		if termination == "active append" {
			Expect(stream.closed).NotTo(BeClosed())
			Expect(stream.callSnapshot()).To(Equal([]string{"append"}))
			release()
		}
		expectClosedStream(stream, "append", "close")
		if termination != "clear" {
			Eventually(done, asrTestTimeout).Should(BeClosed())
		}
		if termination != "active append" {
			Expect(results).NotTo(Receive())
		}
	}, Entry("clear", "clear"), Entry("cancel", "cancel"), Entry("active append", "active append"), Entry("undelivered result", "undelivered result"), Entry("channel close", "channel close"))
})
