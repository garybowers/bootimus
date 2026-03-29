# Stage 1: Build the binary
FROM golang:1.25 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/ ./web/
COPY bootloaders/ ./bootloaders/
COPY main.go .

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -a -ldflags="-w -s -X bootimus/internal/server.Version=${VERSION}" \
    -o bootimus .

# Stage 2: Runtime
FROM debian:trixie-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    wimtools \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/bootimus /bootimus

EXPOSE 69/udp 8080/tcp 8081/tcp 10809/tcp

USER root

VOLUME [ "/data" ]
ENTRYPOINT ["/bootimus"]
CMD ["serve"]
