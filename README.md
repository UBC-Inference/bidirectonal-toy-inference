# Bidirectional Toy Inference

This repository will contain a low-latency voice inference system built around
two services:

```text
.
|-- coordinator/
|   |-- go.mod
|   |-- main.go
|   `-- Dockerfile
|-- vllm/
|   |-- pyproject.toml
|   |-- uv.lock
|   |-- main.py
|   `-- Dockerfile
|-- compose.yaml
|-- .env.example
|-- .gitignore
`-- README.md
```

## Coordinator

The Go coordinator will own the client API, session state, and inference
orchestration. It will integrate sherpa-onnx through cgo and stream partial ASR
transcripts to the vLLM service.

## vLLM

The Python service will use the vLLM library to serve Qwen 2.5-7B-Instruct.
Its Python environment and dependencies will be managed with uv.

## Data Flow

```text
client audio
    -> coordinator
    -> sherpa-onnx ASR
    -> coordinator
    -> vLLM
    -> coordinator
    -> client events
```

See the [Bidirectional Inference design](https://ubc-inference.github.io/Quickstart/#/bidirectional-inference)
for project goals and architectural context.
