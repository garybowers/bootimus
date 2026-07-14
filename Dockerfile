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
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -a -ldflags="-w -s -X bootimus/internal/server.Version=${VERSION}" \
    -o /out/bootimus-${TARGETOS}-${TARGETARCH} .

# Alias for runtime stage
RUN cp /out/bootimus-${TARGETOS}-${TARGETARCH} /out/bootimus

# Stage for exporting binaries only
FROM scratch AS binaries
COPY --from=builder /out/ /

# Stage: assemble the zero-enrollment Secure Boot set (Microsoft-signed shim
# + iPXE binaries signed by the iPXE Secure Boot CA) at build time, so the
# image ships it ready to seed into the data volume.
FROM debian:trixie-slim AS secureboot-official

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY scripts/build-secureboot-official-set.sh scripts/
COPY bootloaders/default/undionly.kpxe bootloaders/default/
RUN bash scripts/build-secureboot-official-set.sh

# Stage 2: Runtime
FROM debian:trixie-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    wimtools samba ca-certificates libarchive-tools \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/bootimus /bootimus
COPY --from=secureboot-official /src/bootloaders/secureboot-official /usr/share/bootimus/secureboot-official
COPY scripts/docker-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 69/udp 8080/tcp 8081/tcp 10809/tcp 445/tcp

USER root

VOLUME [ "/data" ]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["serve"]
