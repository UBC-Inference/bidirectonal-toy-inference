package main

import (
	"encoding/json"
	"fmt"
)

const modelID = "Qwen/Qwen2.5-7B-Instruct"

type sessionState struct {
	config sessionConfig
	audio  audioBuffer
	items  []conversationItem
}

type conversationItem struct {
	id         string
	role       string
	audio      []byte
	transcript string
}

type sessionConfig struct {
	ID               string      `json:"id"`
	Object           string      `json:"object"`
	Type             string      `json:"type"`
	Model            string      `json:"model"`
	Instructions     string      `json:"instructions,omitempty"`
	OutputModalities []string    `json:"output_modalities"`
	Audio            audioConfig `json:"audio"`
}

type sessionUpdate struct {
	Type             string      `json:"type"`
	Model            string      `json:"model"`
	Instructions     *string     `json:"instructions"`
	OutputModalities []string    `json:"output_modalities"`
	Audio            audioConfig `json:"audio"`
}

type audioConfig struct {
	Input audioInputConfig `json:"input"`
}

type audioInputConfig struct {
	Format audioFormat `json:"format"`
}

type audioFormat struct {
	Type string `json:"type"`
	Rate int    `json:"rate"`
}

func updateSession(event clientEvent, config *sessionConfig) *serverEvent {
	if len(event.Session) == 0 {
		return errorEvent(event.EventID, "missing_required_parameter", "session is required", "session")
	}

	var update sessionUpdate
	if err := json.Unmarshal(event.Session, &update); err != nil {
		return errorEvent(event.EventID, "invalid_value", "session must be an object", "session")
	}
	if update.Type != "" && update.Type != "realtime" {
		return errorEvent(event.EventID, "invalid_value", "session type must be realtime", "session.type")
	}
	if update.Model != "" && update.Model != modelID {
		return errorEvent(event.EventID, "invalid_value", fmt.Sprintf("model must be %s", modelID), "session.model")
	}
	if update.OutputModalities != nil && (len(update.OutputModalities) != 1 || update.OutputModalities[0] != "text") {
		return errorEvent(event.EventID, "invalid_value", "only text output is supported", "session.output_modalities")
	}
	format := update.Audio.Input.Format
	if format.Type != "" && format.Type != "audio/pcm" {
		return errorEvent(event.EventID, "invalid_value", "audio format must be audio/pcm", "session.audio.input.format.type")
	}
	if format.Rate != 0 && format.Rate != 24000 {
		return errorEvent(event.EventID, "invalid_value", "audio sample rate must be 24000", "session.audio.input.format.rate")
	}
	if update.Instructions != nil {
		config.Instructions = *update.Instructions
	}
	updatedConfig := *config

	return &serverEvent{
		EventID: newID("event"),
		Type:    SessionUpdated,
		Session: &updatedConfig,
	}
}
