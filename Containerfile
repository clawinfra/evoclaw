# ── Build Stage ──────────────────────────────────────────────────
FROM golang:1.24 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /evoclaw ./cmd/evoclaw

# ── Runtime Stage ────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=builder /evoclaw /evoclaw
COPY --from=builder /src/web /web

EXPOSE 8080

ENTRYPOINT ["/evoclaw"]
