package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/asr"
)

func Run(shutdownContext context.Context) error {
	config, err := loadCoordinatorConfig()
	if err != nil {
		return err
	}
	recognizer, err := asr.NewNativeRecognizer(config.asr)
	if err != nil {
		return err
	}
	defer recognizer.Close()
	return serveCoordinator(shutdownContext, config, recognizer)
}

func serveCoordinator(shutdownContext context.Context, config coordinatorConfig, recognizer asr.Recognizer) error {
	sessionContext, cancelSessions := context.WithCancel(shutdownContext)
	defer cancelSessions()
	requests := requestTracker{handler: newRouter(sessionContext, config.apiKey, recognizer)}
	server := &http.Server{
		Addr:              ":" + config.port,
		Handler:           &requests,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listenerDone := make(chan error, 1)
	slog.Info("coordinator listening", "port", config.port)
	go func() { listenerDone <- server.ListenAndServe() }()

	var listenerError error
	listenerExited := false
	select {
	case listenerError = <-listenerDone:
		listenerExited = true
	case <-shutdownContext.Done():
	}
	requests.stopAccepting()
	cancelSessions()
	shutdownError := shutdownHTTPServer(server)
	if !listenerExited {
		listenerError = <-listenerDone
	}
	requests.wait()
	if errors.Is(listenerError, http.ErrServerClosed) {
		listenerError = nil
	}
	if listenerError != nil {
		listenerError = fmt.Errorf("listen: %w", listenerError)
	}
	return errors.Join(listenerError, shutdownError)
}

func shutdownHTTPServer(server *http.Server) error {
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil {
		return errors.Join(err, server.Close())
	}
	return nil
}

type requestTracker struct {
	handler  http.Handler
	mutex    sync.Mutex
	active   sync.WaitGroup
	stopping bool
}

func (requests *requestTracker) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	requests.mutex.Lock()
	if requests.stopping {
		requests.mutex.Unlock()
		http.Error(responseWriter, "server shutting down", http.StatusServiceUnavailable)
		return
	}
	requests.active.Add(1)
	requests.mutex.Unlock()
	defer requests.active.Done()
	requests.handler.ServeHTTP(responseWriter, request)
}

func (requests *requestTracker) stopAccepting() {
	requests.mutex.Lock()
	requests.stopping = true
	requests.mutex.Unlock()
}

func (requests *requestTracker) wait() {
	// HTTP shutdown does not wait for handlers on upgraded WebSocket connections.
	requests.active.Wait()
}
