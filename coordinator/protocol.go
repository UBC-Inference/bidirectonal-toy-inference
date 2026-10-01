package main

import "encoding/json"

type EventType string

const (
	SessionUpdate             EventType = "session.update"
	SessionCreated            EventType = "session.created"
	SessionUpdated            EventType = "session.updated"
	InputAudioBufferAppend    EventType = "input_audio_buffer.append"
	InputAudioBufferCommit    EventType = "input_audio_buffer.commit"
	InputAudioBufferCommitted EventType = "input_audio_buffer.committed"
	InputAudioBufferClear     EventType = "input_audio_buffer.clear"
	InputAudioBufferCleared   EventType = "input_audio_buffer.cleared"
	Error                     EventType = "error"
)

type clientEvent struct {
	EventID string          `json:"event_id,omitempty"`
	Type    EventType       `json:"type"`
	Audio   string          `json:"audio,omitempty"`
	Session json.RawMessage `json:"session,omitempty"`
}

type serverEvent struct {
	EventID        string         `json:"event_id"`
	Type           EventType      `json:"type"`
	Session        *sessionConfig `json:"session,omitempty"`
	PreviousItemID *string        `json:"previous_item_id,omitempty"`
	ItemID         string         `json:"item_id,omitempty"`
	Error          *eventError    `json:"error,omitempty"`
}

type eventError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
	EventID string `json:"event_id,omitempty"`
}

func errorEvent(clientEventID, code, message, param string) *serverEvent {
	return &serverEvent{
		EventID: newID("event"),
		Type:    Error,
		Error: &eventError{
			Type:    "invalid_request_error",
			Code:    code,
			Message: message,
			Param:   param,
			EventID: clientEventID,
		},
	}
}
