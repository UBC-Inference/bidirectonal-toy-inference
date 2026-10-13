package server

import (
	"errors"
	"os"

	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/asr"
)

type coordinatorConfig struct {
	port   string
	apiKey string
	asr    asr.Config
}

func loadCoordinatorConfig() (coordinatorConfig, error) {
	config := coordinatorConfig{
		port:   os.Getenv("PORT"),
		apiKey: os.Getenv("COORDINATOR_API_KEY"),
		asr: asr.Config{
			EncoderPath: os.Getenv("ASR_ENCODER_PATH"),
			DecoderPath: os.Getenv("ASR_DECODER_PATH"),
			JoinerPath:  os.Getenv("ASR_JOINER_PATH"),
			TokensPath:  os.Getenv("ASR_TOKENS_PATH"),
		},
	}
	if config.port == "" {
		config.port = "8080"
	}
	if config.apiKey == "" {
		return coordinatorConfig{}, errors.New("COORDINATOR_API_KEY is required")
	}
	return config, nil
}
