package main

type asrRecognizer interface {
	NewStream() (asrStream, error)
}

type asrStream interface {
	Append([]float32) (string, error)
	Finish() (string, error)
	Close()
}
