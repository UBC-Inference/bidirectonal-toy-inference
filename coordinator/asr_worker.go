package main

import (
	"context"
	"fmt"
)

type asrCommandType int

const (
	// Feed audio and return the current transcript for an unfinished turn.
	asrAppendAudio asrCommandType = iota + 1
	// Finish transcription and release the turn's stream, not the session worker.
	asrFinishTurn
	// Release pending recognition state without completing a transcript.
	asrDiscardTurn
)

type asrCommand struct {
	kind        asrCommandType
	audioItemID string // Conversation item reserved on first append and retained on commit.
	samples     []float32
}

// Failures travel with the item identity because recognition runs outside the session owner.
type transcriptionResult struct {
	audioItemID  string
	transcript   string
	turnFinished bool // Ends this turn, including failures, but not the session worker.
	err          error
}

type asrWorker struct {
	recognizer  asrRecognizer
	stream      asrStream
	audioItemID string
	streamError error
}

func runASRWorker(sessionContext context.Context, recognizer asrRecognizer, commands <-chan asrCommand, results chan<- transcriptionResult, done chan<- struct{}) {
	defer close(done)
	worker := asrWorker{recognizer: recognizer}
	defer worker.closeStream()
	for {
		var command asrCommand
		select {
		case <-sessionContext.Done():
			return
		case incomingCommand, open := <-commands:
			if !open {
				return
			}
			command = incomingCommand
		}
		if sessionContext.Err() != nil {
			return
		}
		result := worker.handleCommand(command)
		if result == nil {
			continue
		}
		select {
		case results <- *result:
		case <-sessionContext.Done():
			return
		}
	}
}

func (worker *asrWorker) handleCommand(command asrCommand) *transcriptionResult {
	switch command.kind {
	case asrDiscardTurn:
		worker.discardTurn()
		return nil
	case asrAppendAudio, asrFinishTurn:
		return worker.transcribe(command)
	default:
		return &transcriptionResult{audioItemID: command.audioItemID, err: fmt.Errorf("unsupported ASR command: %d", command.kind)}
	}
}

func (worker *asrWorker) transcribe(command asrCommand) *transcriptionResult {
	result := &transcriptionResult{audioItemID: command.audioItemID, turnFinished: command.kind == asrFinishTurn}
	if command.audioItemID == "" {
		result.err = fmt.Errorf("ASR command requires an audio item ID")
		return result
	}
	if worker.audioItemID != "" && command.audioItemID != worker.audioItemID {
		result.err = fmt.Errorf("ASR command for %s arrived before %s finished", command.audioItemID, worker.audioItemID)
		return result
	}
	if worker.audioItemID == "" {
		worker.audioItemID = command.audioItemID
		worker.stream, worker.streamError = worker.recognizer.NewStream()
	}
	if worker.streamError == nil {
		switch command.kind {
		case asrAppendAudio:
			result.transcript, worker.streamError = worker.stream.Append(command.samples)
		case asrFinishTurn:
			result.transcript, worker.streamError = worker.stream.Finish()
		}
	}
	result.err = worker.streamError
	if result.turnFinished {
		worker.discardTurn()
	} else if result.err != nil {
		worker.closeStream()
	}
	return result
}

func (worker *asrWorker) closeStream() {
	if worker.stream != nil {
		worker.stream.Close()
		worker.stream = nil
	}
}

func (worker *asrWorker) discardTurn() {
	worker.closeStream()
	worker.audioItemID = ""
	worker.streamError = nil
}
