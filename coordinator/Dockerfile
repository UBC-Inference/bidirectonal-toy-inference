FROM golang:1.24-bookworm AS build

ARG TARGETARCH
ENV CGO_ENABLED=1

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /coordinator ./cmd/coordinator
RUN case "$TARGETARCH" in \
      amd64) native_arch=x86_64-unknown-linux-gnu ;; \
      arm64) native_arch=aarch64-unknown-linux-gnu ;; \
      *) echo "Unsupported TARGETARCH: $TARGETARCH" >&2; exit 1 ;; \
    esac && \
    native_dir="$(go list -m -f '{{.Dir}}' github.com/k2-fsa/sherpa-onnx-go-linux)" && \
    mkdir -p /native-libs && \
    cp -a "$native_dir/lib/$native_arch/"*.so* /native-libs/

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates libstdc++6 wget && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /native-libs/ /usr/local/lib/
COPY --from=build /coordinator /usr/local/bin/coordinator
ENV LD_LIBRARY_PATH=/usr/local/lib

EXPOSE 8080
CMD ["coordinator"]
