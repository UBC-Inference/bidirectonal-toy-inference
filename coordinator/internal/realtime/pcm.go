package realtime

import (
	"encoding/binary"
	"fmt"
)

const pcm16Scale = 1 << 15

func pcm16ToFloat(audio []byte) ([]float32, error) {
	if len(audio)%2 != 0 {
		return nil, fmt.Errorf("PCM16 audio must contain complete samples")
	}
	samples := make([]float32, len(audio)/2)
	for index := range samples {
		offset := index * 2
		sampleBits := binary.LittleEndian.Uint16(audio[offset : offset+2])
		signedSample := int16(sampleBits)
		samples[index] = float32(signedSample) / float32(pcm16Scale)
	}
	return samples, nil
}
