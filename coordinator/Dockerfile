FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /coordinator .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates
COPY --from=build /coordinator /usr/local/bin/coordinator

EXPOSE 8080
CMD ["coordinator"]
