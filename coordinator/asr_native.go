package main

import (
	"errors"
	"fmt"
	"os"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

type nativeRecognizer struct {
	recognizer *sherpa.OnlineRecognizer
}

type asrConfig struct {
	EncoderPath string
	DecoderPath string
	JoinerPath  string
	TokensPath  string
}

type nativeStream struct {
	recognizer *sherpa.OnlineRecognizer
	stream     *sherpa.OnlineStream
	finished   bool
}

func newNativeRecognizer(config asrConfig) (*nativeRecognizer, error) {
	for _, model := range []struct{ name, path string }{
		{"encoder", config.EncoderPath},
		{"decoder", config.DecoderPath},
		{"joiner", config.JoinerPath},
		{"tokens", config.TokensPath},
	} {
		if model.path == "" {
			return nil, fmt.Errorf("%s path is required", model.name)
		}
		file, err := os.Open(model.path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", model.name, err)
		}
		info, err := file.Stat()
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", model.name, err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, fmt.Errorf("%s must be a nonempty regular file", model.name)
		}
	}

	nativeConfig := sherpa.OnlineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{SampleRate: 16000, FeatureDim: 80},
		ModelConfig: sherpa.OnlineModelConfig{
			Transducer: sherpa.OnlineTransducerModelConfig{
				Encoder: config.EncoderPath,
				Decoder: config.DecoderPath,
				Joiner:  config.JoinerPath,
			},
			Tokens:     config.TokensPath,
			NumThreads: 1,
			Provider:   "cpu",
			ModelType:  "zipformer",
		},
		DecodingMethod: "greedy_search",
		EnableEndpoint: 0,
	}
	recognizer := sherpa.NewOnlineRecognizer(&nativeConfig)
	if recognizer == nil {
		return nil, errors.New("initialize native ASR recognizer failed")
	}
	return &nativeRecognizer{recognizer: recognizer}, nil
}

func (r *nativeRecognizer) NewStream() (asrStream, error) {
	if r.recognizer == nil {
		return nil, errors.New("ASR recognizer is closed")
	}
	// The binding does not expose whether the stream's native pointer is nil.
	stream := sherpa.NewOnlineStream(r.recognizer)
	stream.AcceptWaveform(24000, make([]float32, 24000/2))
	return &nativeStream{
		recognizer: r.recognizer,
		stream:     stream,
	}, nil
}

func (r *nativeRecognizer) Close() {
	if r.recognizer != nil {
		sherpa.DeleteOnlineRecognizer(r.recognizer)
		r.recognizer = nil
	}
}

func (s *nativeStream) Append(samples []float32) (string, error) {
	if s.stream == nil {
		return "", errors.New("ASR stream is closed")
	}
	if s.finished {
		return "", errors.New("ASR stream is finished")
	}
	for offset := 0; offset < len(samples); offset += 2400 {
		s.stream.AcceptWaveform(24000, samples[offset:min(offset+2400, len(samples))])
		for s.recognizer.IsReady(s.stream) {
			s.recognizer.Decode(s.stream)
		}
	}
	return s.recognizer.GetResult(s.stream).Text, nil
}

func (s *nativeStream) Finish() (string, error) {
	if s.stream == nil {
		return "", errors.New("ASR stream is closed")
	}
	if !s.finished {
		s.stream.AcceptWaveform(24000, make([]float32, 24000*8/10))
		s.stream.InputFinished()
		s.finished = true
	}
	for s.recognizer.IsReady(s.stream) {
		s.recognizer.Decode(s.stream)
	}
	return s.recognizer.GetResult(s.stream).Text, nil
}

func (s *nativeStream) Close() {
	if s.stream != nil {
		sherpa.DeleteOnlineStream(s.stream)
		s.stream = nil
	}
}
