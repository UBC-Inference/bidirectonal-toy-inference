package main

import "encoding/base64"

const maxBufferedAudioSize = 24000 * 2 * 60

type audioBuffer struct {
	itemID     string
	bytes      int
	transcript string
	revision   int
	failed     bool
}

func decodeAudio(event clientEvent, bufferedBytes int) ([]float32, *serverEvent) {
	if event.Audio == "" {
		return nil, errorEvent(event.EventID, "missing_required_parameter", "audio is required", "audio")
	}
	audio, err := base64.StdEncoding.DecodeString(event.Audio)
	if err != nil {
		return nil, errorEvent(event.EventID, "invalid_value", "audio must be valid base64", "audio")
	}
	if len(audio) == 0 {
		return nil, errorEvent(event.EventID, "invalid_value", "audio must contain at least one PCM16 sample", "audio")
	}
	if bufferedBytes+len(audio) > maxBufferedAudioSize {
		return nil, errorEvent(event.EventID, "buffer_too_large", "audio buffer exceeds 60 seconds", "audio")
	}
	samples, err := pcm16ToFloat(audio)
	if err != nil {
		return nil, errorEvent(event.EventID, "invalid_value", err.Error(), "audio")
	}
	return samples, nil
}

func commitAudio(event clientEvent, state *sessionState) *serverEvent {
	if state.audio.bytes == 0 {
		return errorEvent(event.EventID, "input_audio_buffer_commit_empty", "audio buffer is empty", "audio")
	}
	var previousItemID *string
	if len(state.items) > 0 {
		previousID := state.items[len(state.items)-1].id
		previousItemID = &previousID
	}
	response := &serverEvent{EventID: newID("event"), Type: InputAudioBufferCommitted, PreviousItemID: previousItemID, ItemID: state.audio.itemID}
	if state.awaitingTranscription == nil {
		state.awaitingTranscription = make(map[string]int)
	}
	state.awaitingTranscription[state.audio.itemID] = len(state.items)
	state.items = append(state.items, conversationItem{
		id: state.audio.itemID, role: "user", transcript: state.audio.transcript,
		revision: state.audio.revision, failed: state.audio.failed,
	})
	state.audio = audioBuffer{}
	return response
}
