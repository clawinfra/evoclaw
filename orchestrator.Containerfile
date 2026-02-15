# ── Build Stage ──────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=beta -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /evoclaw ./cmd/evoclaw

# ── Runtime Stage ────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -u 1000 evoclaw

WORKDIR /app
COPY --from=builder /evoclaw .
RUN mkdir -p /app/data && chown -R evoclaw:evoclaw /app

USER evoclaw

EXPOSE 8420

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8420/api/status || exit 1

ENTRYPOINT ["./evoclaw"]
CMD ["--config", "/app/evoclaw.json"]
