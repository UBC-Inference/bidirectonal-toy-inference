package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		slog.Error("coordinator failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	shutdownContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	config, err := loadCoordinatorConfig()
	if err != nil {
		return err
	}
	recognizer, err := newNativeRecognizer(config.asr)
	if err != nil {
		return err
	}
	defer recognizer.Close()
	return serveCoordinator(shutdownContext, config, recognizer)
}
