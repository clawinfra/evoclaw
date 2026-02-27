# ── Build Stage ──────────────────────────────────────────────────
FROM rust:1.84-alpine AS builder

RUN apk add --no-cache musl-dev pkgconfig openssl-dev openssl-libs-static

WORKDIR /src
COPY Cargo.toml Cargo.lock ./
# Cache dependencies by building a dummy main
RUN mkdir src && echo "fn main() {}" > src/main.rs && \
    cargo build --release 2>/dev/null || true && \
    rm -rf src

COPY . .
# Touch main.rs so cargo rebuilds the actual binary
RUN touch src/main.rs && \
    cargo build --release

# ── Runtime Stage ────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -u 1000 evoclaw

WORKDIR /app
COPY --from=builder /src/target/release/evoclaw-agent .
RUN mkdir -p /app/keys && chown -R evoclaw:evoclaw /app

USER evoclaw

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD test -f /tmp/evoclaw-agent.pid || exit 1

ENTRYPOINT ["./evoclaw-agent"]
CMD ["--config", "/app/agent.toml"]
