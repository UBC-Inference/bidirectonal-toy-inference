package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/gorilla/websocket"
)

func main() {
	url := flag.String("url", "ws://localhost:8080/v1/realtime", "coordinator WebSocket URL")
	wav := flag.String("wav", "", "PCM16 little-endian mono 24 kHz WAV file")
	mic := flag.Bool("mic", false, "capture from the default microphone until Enter is pressed")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *url, *wav, *mic); err != nil {
		fmt.Fprintln(os.Stderr, "preview:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, url, path string, mic bool) error {
	if mic == (path != "") || flag.NArg() != 0 {
		return errors.New("usage: preview [-url URL] (-wav FILE | -mic)")
	}
	key := os.Getenv("COORDINATOR_API_KEY")
	if key == "" {
		return errors.New("COORDINATOR_API_KEY is required")
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, response, err := dialer.DialContext(ctx, url, http.Header{"Authorization": {"Bearer " + key}})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	stopClosing := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClosing()
	conn.SetReadLimit(4 << 20)
	done := make(chan error, 1)
	go func() {
		defer close(done)
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				done <- fmt.Errorf("read: %w", err)
				return
			}
			var event struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(payload, &event); err != nil {
				done <- fmt.Errorf("decode event: %w", err)
				return
			}
			switch event.Type {
			case "conversation.item.input_audio_transcription.updated",
				"conversation.item.input_audio_transcription.completed":
				if _, err := fmt.Fprintln(os.Stdout, string(payload)); err != nil {
					done <- err
					return
				}
				if event.Type == "conversation.item.input_audio_transcription.completed" {
					done <- nil
					return
				}
			case "error", "conversation.item.input_audio_transcription.failed":
				fmt.Fprintln(os.Stderr, string(payload))
				done <- fmt.Errorf("server sent %s", event.Type)
				return
			}
		}
	}()
	defer func() {
		conn.Close()
		<-done
	}()

	write := func(event any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		return conn.WriteJSON(event)
	}
	appendAudio := func(chunk []byte) error {
		return write(struct {
			Type  string `json:"type"`
			Audio string `json:"audio"`
		}{"input_audio_buffer.append", base64.StdEncoding.EncodeToString(chunk)})
	}
	if err := streamInput(ctx, done, path, mic, appendAudio); err != nil {
		return err
	}
	if err := write(struct {
		Type string `json:"type"`
	}{"input_audio_buffer.commit"}); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	case <-timer.C:
		return errors.New("timed out waiting for transcription completion (30s after commit)")
	}
}

func streamInput(ctx context.Context, done <-chan error, path string, mic bool, appendAudio func([]byte) error) error {
	if mic {
		return streamMicrophone(ctx, done, appendAudio)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	audio, err := readWAV(file)
	if err != nil {
		return fmt.Errorf("WAV: %w", err)
	}
	return streamWAV(ctx, done, audio, appendAudio)
}

func streamWAV(ctx context.Context, done <-chan error, audio io.Reader, appendAudio func([]byte) error) error {
	chunk := make([]byte, 24000*2/10)
	start := time.Now()
	var sent int64
	for {
		n, err := io.ReadFull(audio, chunk)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return fmt.Errorf("read audio: %w", err)
		}
		if n == 0 {
			return nil
		}
		if err := appendAudio(chunk[:n]); err != nil {
			return fmt.Errorf("append: %w", err)
		}
		sent += int64(n)
		timer := time.NewTimer(time.Until(start.Add(time.Duration(sent) * time.Second / 48000)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case err := <-done:
			timer.Stop()
			return err
		case <-timer.C:
		}
	}
}

func streamMicrophone(ctx context.Context, done <-chan error, appendAudio func([]byte) error) error {
	audio := make(chan []byte, 64)
	overflow := make(chan error, 1)
	captureContext, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("initialize microphone context: %w", err)
	}
	defer captureContext.Free()
	defer captureContext.Uninit()

	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatS16
	config.Capture.Channels = 1
	config.SampleRate = 24000
	device, err := malgo.InitDevice(captureContext.Context, config, malgo.DeviceCallbacks{
		Data: func(_, input []byte, _ uint32) {
			chunk := append([]byte(nil), input...)
			select {
			case audio <- chunk:
			default:
				select {
				case overflow <- errors.New("microphone buffer overflow"):
				default:
				}
			}
		},
	})
	if err != nil {
		return fmt.Errorf("initialize microphone: %w", err)
	}
	defer device.Uninit()
	if err := device.Start(); err != nil {
		return fmt.Errorf("start microphone: %w", err)
	}
	defer device.Stop()

	stop := make(chan struct{}, 1)
	go func() {
		fmt.Fprintln(os.Stderr, "Recording from the default microphone; press Enter to stop.")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		stop <- struct{}{}
	}()
	for {
		select {
		case chunk := <-audio:
			if err := appendAudio(chunk); err != nil {
				return fmt.Errorf("append: %w", err)
			}
		case err := <-overflow:
			return err
		case <-stop:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			return err
		}
	}
}

func readWAV(file *os.File) (*io.SectionReader, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	var header [12]byte
	if _, err := file.ReadAt(header[:], 0); err != nil {
		return nil, err
	}
	if string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return nil, errors.New("expected little-endian RIFF/WAVE")
	}
	end := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	if end < 12 || end > info.Size() {
		return nil, errors.New("invalid or truncated RIFF size")
	}
	var formatSeen, dataSeen bool
	var dataOffset, dataSize int64
	for offset := int64(12); offset < end; {
		var chunk [8]byte
		if end-offset < 8 {
			return nil, errors.New("truncated chunk header")
		}
		if _, err := file.ReadAt(chunk[:], offset); err != nil {
			return nil, err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		offset += 8
		padded := size + size%2
		if padded > end-offset {
			return nil, errors.New("chunk exceeds RIFF boundary")
		}
		switch string(chunk[:4]) {
		case "fmt ":
			formatSeen = true
		case "data":
			if dataSeen || size == 0 || size%2 != 0 {
				return nil, errors.New("duplicate, empty, or sample-misaligned data chunk")
			}
			dataSeen, dataOffset, dataSize = true, offset, size
		}
		offset += padded
	}
	if !formatSeen || !dataSeen {
		return nil, errors.New("missing fmt or data chunk")
	}
	return io.NewSectionReader(file, dataOffset, dataSize), nil
}
