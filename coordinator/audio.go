package main

import "encoding/base64"

const maxBufferedAudioSize = 24000 * 2 * 60

type audioBuffer struct {
	pending []byte
}

func appendAudio(event clientEvent, buffer *audioBuffer) *serverEvent {
	if event.Audio == "" {
		return errorEvent(event.EventID, "missing_required_parameter", "audio is required", "audio")
	}
	audio, err := base64.StdEncoding.DecodeString(event.Audio)
	if err != nil {
		return errorEvent(event.EventID, "invalid_value", "audio must be valid base64", "audio")
	}
	if len(audio)%2 != 0 {
		return errorEvent(event.EventID, "invalid_value", "PCM16 audio must contain complete samples", "audio")
	}
	if len(buffer.pending)+len(audio) > maxBufferedAudioSize {
		return errorEvent(event.EventID, "buffer_too_large", "audio buffer exceeds 60 seconds", "audio")
	}
	buffer.pending = append(buffer.pending, audio...)
	return nil
}

func commitAudio(event clientEvent, state *sessionState) *serverEvent {
	if len(state.audio.pending) == 0 {
		return errorEvent(event.EventID, "input_audio_buffer_commit_empty", "audio buffer is empty", "audio")
	}

	itemID := newID("item")
	var previousItemID *string
	if len(state.items) > 0 {
		previousID := state.items[len(state.items)-1].id
		previousItemID = &previousID
	}
	response := &serverEvent{
		EventID:        newID("event"),
		Type:           InputAudioBufferCommitted,
		PreviousItemID: previousItemID,
		ItemID:         itemID,
	}
	state.items = append(state.items, conversationItem{
		id:    itemID,
		role:  "user",
		audio: state.audio.pending,
	})
	state.audio.pending = nil
	return response
}
