package main

import (
	"net/http"

	"github.com/gorilla/websocket"
)

const maxMessageBytes = 16 << 20

var realtimeUpgrader = websocket.Upgrader{}

func handleRealtime(responseWriter http.ResponseWriter, request *http.Request) {
	connection, err := realtimeUpgrader.Upgrade(responseWriter, request, nil)
	if err != nil {
		return
	}
	connection.SetReadLimit(maxMessageBytes)

	outbound := make(chan serverEvent, 32)
	writerDone := make(chan struct{})
	go writeEvents(connection, outbound, writerDone)
	defer closeSession(connection, outbound, writerDone)

	config := sessionConfig{
		ID:               newID("sess"),
		Object:           "realtime.session",
		Type:             "realtime",
		Model:            modelID,
		OutputModalities: []string{"text"},
		Audio: audioConfig{Input: audioInputConfig{Format: audioFormat{
			Type: "audio/pcm",
			Rate: 24000,
		}}},
	}
	createdConfig := config
	if !enqueueEvent(outbound, writerDone, serverEvent{
		EventID: newID("event"),
		Type:    SessionCreated,
		Session: &createdConfig,
	}) {
		return
	}

	var audio audioBuffer
	var previousItemID *string

	for {
		var event clientEvent
		if err := connection.ReadJSON(&event); err != nil {
			return
		}

		var response *serverEvent
		switch event.Type {
		case SessionUpdate:
			response = updateSession(event, &config)
		case InputAudioBufferAppend:
			response = appendAudio(event, &audio)
		case InputAudioBufferCommit:
			response = commitAudio(event, &audio, &previousItemID)
		case InputAudioBufferClear:
			audio.pending = nil
			response = &serverEvent{EventID: newID("event"), Type: InputAudioBufferCleared}
		default:
			response = errorEvent(event.EventID, "invalid_value", "unsupported event type", "type")
		}

		if response != nil && !enqueueEvent(outbound, writerDone, *response) {
			return
		}
	}
}

func closeSession(connection *websocket.Conn, outbound chan serverEvent, writerDone <-chan struct{}) {
	connection.Close()
	close(outbound)
	<-writerDone
}

func writeEvents(connection *websocket.Conn, events <-chan serverEvent, done chan<- struct{}) {
	defer close(done)
	for event := range events {
		if err := connection.WriteJSON(event); err != nil {
			connection.Close()
			return
		}
	}
}

func enqueueEvent(events chan<- serverEvent, writerDone <-chan struct{}, event serverEvent) bool {
	select {
	case events <- event:
		return true
	case <-writerDone:
		return false
	}
}
