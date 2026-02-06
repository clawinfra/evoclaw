# EvoClaw Edge Agent — E2B Sandbox Template
#
# Minimal image for running the Rust edge agent inside an E2B Firecracker microVM.
# The agent auto-starts when the sandbox boots via the E2B start command.
#
# Build:
#   e2b template build --name evoclaw-agent --dockerfile e2b.Dockerfile
#
# Or manually from repo root:
#   docker build -f deploy/e2b/e2b.Dockerfile -t evoclaw-agent-e2b .

FROM ubuntu:22.04 AS base

# Avoid interactive prompts
ENV DEBIAN_FRONTEND=noninteractive

# Install minimal runtime dependencies + MQTT tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    mosquitto-clients \
    curl \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Create agent user and directories
RUN useradd -r -m -d /home/agent -s /bin/bash agent \
    && mkdir -p /opt/evoclaw /var/lib/evoclaw /var/log/evoclaw \
    && chown -R agent:agent /opt/evoclaw /var/lib/evoclaw /var/log/evoclaw

# Copy the pre-compiled edge agent binary
# Build with: cd edge-agent && cargo build --release --target x86_64-unknown-linux-gnu
COPY edge-agent/target/release/evoclaw-agent /opt/evoclaw/evoclaw-agent
RUN chmod +x /opt/evoclaw/evoclaw-agent

# Copy default configuration
COPY deploy/e2b/agent.toml /opt/evoclaw/agent.toml

# Copy entrypoint script
COPY deploy/e2b/entrypoint.sh /opt/evoclaw/entrypoint.sh
RUN chmod +x /opt/evoclaw/entrypoint.sh

# Set working directory
WORKDIR /opt/evoclaw

# Agent runs as non-root
USER agent

# E2B uses the start command defined in e2b.toml
# The entrypoint handles config injection and agent startup
CMD ["/opt/evoclaw/entrypoint.sh"]
