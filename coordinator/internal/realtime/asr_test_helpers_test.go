package realtime

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/asr"
	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const asrTestTimeout = 3 * time.Second

type asrRecognizer = asr.Recognizer
type asrStream = asr.Stream

type fakeASRRecognizer struct{ newStream func() (asrStream, error) }

func (recognizer fakeASRRecognizer) NewStream() (asrStream, error) { return recognizer.newStream() }

func sequenceRecognizer(streams ...asrStream) fakeASRRecognizer {
	var mutex sync.Mutex
	return fakeASRRecognizer{newStream: func() (asrStream, error) {
		mutex.Lock()
		defer mutex.Unlock()
		if len(streams) == 0 {
			return nil, errors.New("unexpected extra stream")
		}
		stream := streams[0]
		streams = streams[1:]
		return stream, nil
	}}
}

type fakeASRStream struct {
	mu         sync.Mutex
	calls      []string
	samples    [][]float32
	active     atomic.Int32
	concurrent atomic.Bool
	appendFn   func([]float32) (string, error)
	finishFn   func() (string, error)
	closed     chan struct{}
	closeOnce  sync.Once
}

func newFakeASRStream() *fakeASRStream { return &fakeASRStream{closed: make(chan struct{})} }

func (stream *fakeASRStream) enter(call string) func() {
	if stream.active.Add(1) != 1 {
		stream.concurrent.Store(true)
	}
	stream.mu.Lock()
	stream.calls = append(stream.calls, call)
	stream.mu.Unlock()
	return func() { stream.active.Add(-1) }
}

func (stream *fakeASRStream) Append(samples []float32) (string, error) {
	defer stream.enter("append")()
	stream.mu.Lock()
	stream.samples = append(stream.samples, append([]float32(nil), samples...))
	stream.mu.Unlock()
	if stream.appendFn != nil {
		return stream.appendFn(samples)
	}
	return "partial", nil
}

func (stream *fakeASRStream) Finish() (string, error) {
	defer stream.enter("finish")()
	if stream.finishFn != nil {
		return stream.finishFn()
	}
	return "final", nil
}

func (stream *fakeASRStream) Close() {
	defer stream.enter("close")()
	stream.closeOnce.Do(func() { close(stream.closed) })
}

func (stream *fakeASRStream) callSnapshot() []string {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return append([]string(nil), stream.calls...)
}

func expectClosedStream(stream *fakeASRStream, calls ...string) {
	Eventually(stream.closed, asrTestTimeout).Should(BeClosed())
	Expect(stream.callSnapshot()).To(Equal(calls))
	Expect(stream.concurrent.Load()).To(BeFalse())
}

func blockFirstAppend(stream *fakeASRStream) (<-chan struct{}, func()) {
	entered, released := make(chan struct{}), make(chan struct{})
	var appendOnce, releaseOnce sync.Once
	appendFn := stream.appendFn
	stream.appendFn = func(samples []float32) (string, error) {
		appendOnce.Do(func() { close(entered); <-released })
		if appendFn != nil {
			return appendFn(samples)
		}
		return "partial", nil
	}
	release := func() { releaseOnce.Do(func() { close(released) }) }
	DeferCleanup(release)
	return entered, release
}

func startTestWorker(recognizer asrRecognizer, resultCapacity int) (chan asrCommand, chan transcriptionResult, chan struct{}, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	commands, results, done := make(chan asrCommand, 16), make(chan transcriptionResult, resultCapacity), make(chan struct{})
	go runASRWorker(ctx, recognizer, commands, results, done)
	DeferCleanup(func() { cancel(); Eventually(done, asrTestTimeout).Should(BeClosed()) })
	return commands, results, done, cancel
}

func receiveTranscription(results <-chan transcriptionResult) transcriptionResult {
	var result transcriptionResult
	Eventually(results, asrTestTimeout).Should(Receive(&result))
	return result
}

func exchangeASRCommand(commands chan<- asrCommand, results <-chan transcriptionResult, command asrCommand) transcriptionResult {
	commands <- command
	return receiveTranscription(results)
}

func readRealtimeEvent(connection *websocket.Conn) serverEvent {
	Expect(connection.SetReadDeadline(time.Now().Add(asrTestTimeout))).To(Succeed())
	var event serverEvent
	Expect(connection.ReadJSON(&event)).To(Succeed())
	return event
}

func sendRealtimeEvent(connection *websocket.Conn, event clientEvent) {
	Expect(connection.SetWriteDeadline(time.Now().Add(asrTestTimeout))).To(Succeed())
	Expect(connection.WriteJSON(event)).To(Succeed())
}

func sendPCMAppend(connection *websocket.Conn, value int) {
	var pcm [2]byte
	binary.LittleEndian.PutUint16(pcm[:], uint16(value))
	sendRealtimeEvent(connection, clientEvent{Type: InputAudioBufferAppend, Audio: base64.StdEncoding.EncodeToString(pcm[:])})
}

func realtimeTestServer(recognizer asrRecognizer) (func() *websocket.Conn, <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		Handle(ctx, recognizer, writer, request)
		finished <- struct{}{}
	}))
	DeferCleanup(func() { cancel(); server.Close() })
	return func() *websocket.Conn {
		connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { connection.Close() })
		Expect(readRealtimeEvent(connection).Type).To(Equal(SessionCreated))
		return connection
	}, finished
}
