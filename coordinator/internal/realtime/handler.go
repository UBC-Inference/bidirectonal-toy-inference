package realtime

import (
	"context"
	"net/http"
	"time"

	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/asr"
	"github.com/gorilla/websocket"
)

const maxMessageBytes = 16 << 20

var realtimeUpgrader = websocket.Upgrader{}

func Handle(processContext context.Context, recognizer asr.Recognizer, responseWriter http.ResponseWriter, request *http.Request) {
	connection, err := realtimeUpgrader.Upgrade(responseWriter, request, nil)
	if err != nil {
		return
	}
	connection.SetReadLimit(maxMessageBytes)
	sessionContext, cancelSession := context.WithCancel(processContext)
	defer cancelSession()

	outbound := make(chan serverEvent, 32)
	writerDone := make(chan struct{})
	go writeEvents(connection, outbound, writerDone)
	defer closeSession(connection, outbound, writerDone)

	readerDone := make(chan struct{})
	clientEvents := make(chan clientEvent, 16)
	go readClientEvents(sessionContext, connection, clientEvents, readerDone, cancelSession)
	stopClosing := context.AfterFunc(sessionContext, func() { connection.Close() })
	defer stopClosing()

	commands := make(chan asrCommand, 16)
	results := make(chan transcriptionResult, 16)
	workerDone := make(chan struct{})
	go runASRWorker(sessionContext, recognizer, commands, results, workerDone)
	defer func() {
		cancelSession()
		connection.Close()
		<-readerDone
		<-workerDone
	}()

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

	state := sessionState{config: config}
	var pendingCommand *asrCommand

	for {
		var response *serverEvent
		incoming := (<-chan clientEvent)(clientEvents)
		var commandOutput chan<- asrCommand
		var command asrCommand
		// Keep consuming ASR results while command backpressure pauses client input.
		if pendingCommand != nil {
			incoming = nil
			commandOutput = commands
			command = *pendingCommand
		}
		select {
		case <-sessionContext.Done():
			return
		case result := <-results:
			response = state.handleTranscription(result)
		case commandOutput <- command:
			pendingCommand = nil
		case event := <-incoming:
			pendingCommand, response = state.handleClientEvent(event)
		}

		if response != nil && !enqueueEvent(outbound, writerDone, *response) {
			return
		}
	}
}

func readClientEvents(sessionContext context.Context, connection *websocket.Conn, events chan<- clientEvent, done chan<- struct{}, cancelSession context.CancelFunc) {
	defer close(done)
	defer cancelSession()
	for {
		var event clientEvent
		if err := connection.ReadJSON(&event); err != nil {
			return
		}
		// A full inbox pauses reading, but cancellation must still unblock the send.
		select {
		case events <- event:
		case <-sessionContext.Done():
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
		connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
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
