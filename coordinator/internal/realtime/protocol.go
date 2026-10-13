package realtime

import "encoding/json"

type RealtimeEventType string

const (
	SessionUpdate             RealtimeEventType = "session.update"
	SessionCreated            RealtimeEventType = "session.created"
	SessionUpdated            RealtimeEventType = "session.updated"
	InputAudioBufferAppend    RealtimeEventType = "input_audio_buffer.append"
	InputAudioBufferCommit    RealtimeEventType = "input_audio_buffer.commit"
	InputAudioBufferCommitted RealtimeEventType = "input_audio_buffer.committed"
	InputAudioBufferClear     RealtimeEventType = "input_audio_buffer.clear"
	InputAudioBufferCleared   RealtimeEventType = "input_audio_buffer.cleared"
	RealtimeError             RealtimeEventType = "error"
	TranscriptionUpdated      RealtimeEventType = "conversation.item.input_audio_transcription.updated"
	TranscriptionCompleted    RealtimeEventType = "conversation.item.input_audio_transcription.completed"
	TranscriptionFailed       RealtimeEventType = "conversation.item.input_audio_transcription.failed"
)

type clientEvent struct {
	EventID string            `json:"event_id,omitempty"`
	Type    RealtimeEventType `json:"type"`
	Audio   string            `json:"audio,omitempty"`
	Session json.RawMessage   `json:"session,omitempty"`
}

type serverEvent struct {
	EventID        string            `json:"event_id"`
	Type           RealtimeEventType `json:"type"`
	Session        *sessionConfig    `json:"session,omitempty"`
	PreviousItemID *string           `json:"previous_item_id,omitempty"`
	ItemID         string            `json:"item_id,omitempty"`
	Error          *eventError       `json:"error,omitempty"`
	Transcript     *string           `json:"transcript,omitempty"`
	ContentIndex   *int              `json:"content_index,omitempty"`
	Revision       int               `json:"revision,omitempty"`
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
		Type:    RealtimeError,
		Error: &eventError{
			Type:    "invalid_request_error",
			Code:    code,
			Message: message,
			Param:   param,
			EventID: clientEventID,
		},
	}
}
