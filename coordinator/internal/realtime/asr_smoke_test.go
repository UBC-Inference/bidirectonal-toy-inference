//go:build asr_smoke

package realtime

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/asr"
)

func TestNativeASRSmoke(t *testing.T) {
	for _, name := range []string{"ASR_ENCODER_PATH", "ASR_DECODER_PATH", "ASR_JOINER_PATH", "ASR_TOKENS_PATH"} {
		if os.Getenv(name) == "" {
			t.Skipf("native smoke test requires %s", name)
		}
	}
	recognizer, err := asr.NewNativeRecognizer(asr.Config{
		EncoderPath: os.Getenv("ASR_ENCODER_PATH"),
		DecoderPath: os.Getenv("ASR_DECODER_PATH"),
		JoinerPath:  os.Getenv("ASR_JOINER_PATH"),
		TokensPath:  os.Getenv("ASR_TOKENS_PATH"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer recognizer.Close()

	t.Run("24k silence and fresh streams", func(t *testing.T) {
		for turn := 0; turn < 2; turn++ {
			stream, err := recognizer.NewStream()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(stream.Close)
			for chunk := 0; chunk < 10; chunk++ {
				if _, err := stream.Append(make([]float32, 2400)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := stream.Finish(); err != nil {
				t.Fatal(err)
			}
			stream.Close()
		}
	})

	t.Run("optional raw PCM16 speech", func(t *testing.T) {
		path := os.Getenv("ASR_SMOKE_PCM_PATH")
		if path == "" {
			t.Skip("set ASR_SMOKE_PCM_PATH to raw mono little-endian PCM16 at 24000 Hz")
		}
		audio, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		samples, err := pcm16ToFloat(audio)
		if err != nil {
			t.Fatal(err)
		}
		if len(samples) == 0 {
			t.Fatal("speech fixture is empty")
		}
		stream, err := recognizer.NewStream()
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		for offset := 0; offset < len(samples); offset += 2400 {
			end := min(offset+2400, len(samples))
			if _, err := stream.Append(samples[offset:end]); err != nil {
				t.Fatal(err)
			}
		}
		transcript, err := stream.Finish()
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(transcript) == "" {
			t.Fatal("speech fixture produced an empty transcript")
		}
		if expected := os.Getenv("ASR_SMOKE_EXPECT_TRANSCRIPT"); expected != "" && !strings.Contains(strings.ToLower(transcript), strings.ToLower(expected)) {
			t.Fatalf("transcript %q does not contain %q", transcript, expected)
		}
		t.Logf("transcript: %q", transcript)
	})

	t.Run("concurrent independent streams", func(t *testing.T) {
		errors := make(chan error, 2)
		var workers sync.WaitGroup
		for session := 0; session < 2; session++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				stream, err := recognizer.NewStream()
				if err != nil {
					errors <- err
					return
				}
				defer stream.Close()
				for chunk := 0; chunk < 10; chunk++ {
					if _, err = stream.Append(make([]float32, 2400)); err != nil {
						errors <- err
						return
					}
				}
				_, err = stream.Finish()
				errors <- err
			}()
		}
		workers.Wait()
		for session := 0; session < 2; session++ {
			if err := <-errors; err != nil {
				t.Error(err)
			}
		}
	})
}
