FROM golang:1.24-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/kb-tool ./cmd/kb-tool

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --shell /usr/sbin/nologin kbtool \
    && mkdir -p /workspace \
    && chown kbtool:kbtool /workspace

COPY --from=builder /out/kb-tool /usr/local/bin/kb-tool

WORKDIR /workspace
USER kbtool
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/kb-tool"]
CMD ["server", "-addr", "0.0.0.0:8080"]
