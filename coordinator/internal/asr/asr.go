package asr

type Recognizer interface {
	NewStream() (Stream, error)
}

type Stream interface {
	Append([]float32) (string, error)
	Finish() (string, error)
	Close()
}
