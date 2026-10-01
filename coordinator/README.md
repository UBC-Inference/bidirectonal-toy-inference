# Coordinator

This document specifies the coordinator's external interface and runtime
behavior. See the [Bidirectional Inference design](https://ubc-inference.github.io/Quickstart/#/bidirectional-inference)
for the system architecture and project goals.

## Client API

`GET /v1/realtime` upgrades an authenticated HTTP request to a WebSocket. One
connection owns one in-memory session. Client and server messages use the
OpenAI Realtime JSON event format.

Clients authenticate the upgrade with a bearer token:

```text
Authorization: Bearer $COORDINATOR_API_KEY
```

The initial protocol accepts these client events:

- `session.update`
- `input_audio_buffer.append`
- `input_audio_buffer.commit`
- `input_audio_buffer.clear`

It emits these server events:

- `session.created`
- `session.updated`
- `input_audio_buffer.committed`
- `input_audio_buffer.cleared`
- `error`

Unsupported events produce an `error` event without closing the session.

## Audio

Audio is base64-encoded PCM16, 24 kHz, mono, and little-endian. Clients append
audio chunks with `input_audio_buffer.append`. Appends do not produce an
acknowledgement. A commit finalizes the current audio turn and a clear discards
it.

Sherpa-onnx will consume decoded chunks as they arrive and emit partial
transcript revisions. It will run in the coordinator process through cgo.

## Generation

Partial transcripts will start speculative vLLM requests. A newer transcript
revision cancels obsolete work through its request context. Committing a turn
selects or replaces the speculative result using the final transcript. A
subsequent `response.create` will expose that result to the client.

The coordinator will call the Python service over streaming HTTP and translate
its output into OpenAI Realtime `response.*` events. vLLM transport details are
specified by the Python service rather than exposed to clients.

## Session Concurrency

Each session has one WebSocket reader and one writer. All ASR and generation
producers publish typed events to the writer, which is the only goroutine that
writes to the connection. Closing the socket cancels all work owned by that
session.

## Configuration

- `PORT`: HTTP listen port; defaults to `8080`.
- `COORDINATOR_API_KEY`: bearer token required by `/v1/realtime`.
- `VLLM_BASE_URL`: endpoint for the planned vLLM client; use
  `http://vllm:8000` in Compose or `http://localhost:8000` when running both services
  locally. The coordinator will read and validate this setting when the client
  is implemented.

`GET /healthz` remains unauthenticated for container health checks.

Compose requires non-empty `COORDINATOR_PORT`, `COORDINATOR_API_KEY`, and
`VLLM_BASE_URL` settings. Missing or empty values prevent startup.
