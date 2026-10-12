package main

import "fmt"

func (state *sessionState) handleClientEvent(event clientEvent) (*asrCommand, *serverEvent) {
	switch event.Type {
	case SessionUpdate:
		return nil, updateSession(event, &state.config)
	case InputAudioBufferAppend:
		samples, response := decodeAudio(event, state.audio.bytes)
		if response != nil {
			return nil, response
		}
		if state.audio.failed {
			return nil, errorEvent(event.EventID, "transcription_failed", "clear or commit the failed turn before appending new audio", "audio")
		}
		if state.audio.itemID == "" {
			state.nextItemID++
			state.audio.itemID = fmt.Sprintf("item_%d", state.nextItemID)
		}
		state.audio.bytes += len(samples) * 2
		return &asrCommand{kind: asrAppendAudio, audioItemID: state.audio.itemID, samples: samples}, nil
	case InputAudioBufferCommit:
		if state.audio.bytes == 0 {
			return nil, commitAudio(event, state)
		}
		command := &asrCommand{kind: asrFinishTurn, audioItemID: state.audio.itemID}
		return command, commitAudio(event, state)
	case InputAudioBufferClear:
		command := &asrCommand{kind: asrDiscardTurn, audioItemID: state.audio.itemID}
		response := &serverEvent{EventID: newID("event"), Type: InputAudioBufferCleared, ItemID: state.audio.itemID}
		state.audio = audioBuffer{}
		return command, response
	default:
		return nil, errorEvent(event.EventID, "invalid_value", "unsupported event type", "type")
	}
}
