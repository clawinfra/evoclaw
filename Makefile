# EvoClaw — Container Orchestration Makefile
#
# Podman-first, Docker-compatible.
#
# Usage:
#   make up          — Start all services (Podman, default)
#   make up-docker   — Start all services (Docker fallback)
#   make down        — Stop all services
#   make build       — Build container images
#   make logs        — Tail logs
#   make status      — Show running containers
#   make pod-up      — Create Podman pod (alternative to compose)
#   make pod-down    — Tear down Podman pod

# ── Auto-detect runtime ─────────────────────────────────────────
PODMAN_COMPOSE := $(shell command -v podman-compose 2>/dev/null)
DOCKER_COMPOSE := $(shell command -v docker-compose 2>/dev/null || docker compose version >/dev/null 2>&1 && echo "docker compose")

ifdef PODMAN_COMPOSE
  COMPOSE := podman-compose
  RUNTIME := podman
else ifdef DOCKER_COMPOSE
  COMPOSE := docker compose
  RUNTIME := docker
else
  COMPOSE := $(error No container compose tool found. Install podman-compose or docker compose.)
  RUNTIME := none
endif

COMPOSE_FILE      ?= docker-compose.yml
COMPOSE_DEV_FILE  ?= docker-compose.dev.yml
PROJECT           ?= evoclaw

.PHONY: help up up-docker up-dev down build logs status \
        pod-up pod-down clean shell-orchestrator shell-agent \
        build-orchestrator build-agent

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Compose-based targets ───────────────────────────────────────

up: ## Start all services (auto-detects Podman or Docker)
	$(COMPOSE) -f $(COMPOSE_FILE) -p $(PROJECT) up -d
	@echo "\n✅ EvoClaw is running ($(RUNTIME))"
	@echo "   Orchestrator: http://localhost:8420/api/status"
	@echo "   MQTT Broker:  localhost:1883"

up-docker: ## Start all services (force Docker)
	docker compose -f $(COMPOSE_FILE) -p $(PROJECT) up -d
	@echo "\n✅ EvoClaw is running (docker)"

up-dev: ## Start dev environment with hot reload
	$(COMPOSE) -f $(COMPOSE_DEV_FILE) -p $(PROJECT)-dev up
	@echo "\n✅ EvoClaw dev environment started"

down: ## Stop all services
	$(COMPOSE) -f $(COMPOSE_FILE) -p $(PROJECT) down

down-dev: ## Stop dev services
	$(COMPOSE) -f $(COMPOSE_DEV_FILE) -p $(PROJECT)-dev down

build: build-orchestrator build-agent ## Build all container images

build-orchestrator: ## Build orchestrator image
	$(RUNTIME) build -t evoclaw-orchestrator -f orchestrator.Dockerfile .

build-agent: ## Build edge-agent image
	$(RUNTIME) build -t evoclaw-edge-agent -f edge-agent/Dockerfile ./edge-agent

logs: ## Tail logs from all services
	$(COMPOSE) -f $(COMPOSE_FILE) -p $(PROJECT) logs -f

logs-orchestrator: ## Tail orchestrator logs
	$(COMPOSE) -f $(COMPOSE_FILE) -p $(PROJECT) logs -f orchestrator

logs-agent: ## Tail edge-agent logs
	$(COMPOSE) -f $(COMPOSE_FILE) -p $(PROJECT) logs -f edge-agent

status: ## Show running containers
	$(COMPOSE) -f $(COMPOSE_FILE) -p $(PROJECT) ps

restart: ## Restart all services
	$(COMPOSE) -f $(COMPOSE_FILE) -p $(PROJECT) restart

# ── Podman Pod targets (alternative to compose) ────────────────

pod-up: ## Create EvoClaw Podman pod
	@bash deploy/podman-pod.sh up

pod-down: ## Tear down EvoClaw Podman pod
	@bash deploy/podman-pod.sh down

pod-status: ## Show Podman pod status
	@bash deploy/podman-pod.sh status

# ── Utility targets ─────────────────────────────────────────────

shell-orchestrator: ## Shell into orchestrator container
	$(RUNTIME) exec -it evoclaw-orchestrator /bin/sh

shell-agent: ## Shell into edge-agent container
	$(RUNTIME) exec -it evoclaw-edge-agent /bin/sh

clean: ## Remove all EvoClaw containers, images, and volumes
	-$(COMPOSE) -f $(COMPOSE_FILE) -p $(PROJECT) down -v --rmi local
	-$(RUNTIME) rmi evoclaw-orchestrator evoclaw-edge-agent 2>/dev/null
	@echo "🧹 Cleaned up EvoClaw containers and images"

# ── Systemd integration ─────────────────────────────────────────

install-systemd: ## Install systemd service files (requires sudo)
	sudo cp deploy/systemd/evoclaw-orchestrator.service /etc/systemd/system/
	sudo cp deploy/systemd/evoclaw-edge-agent.service /etc/systemd/system/
	sudo cp deploy/systemd/evoclaw-mosquitto.service /etc/systemd/system/
	sudo systemctl daemon-reload
	@echo "✅ Systemd services installed. Enable with:"
	@echo "   sudo systemctl enable --now evoclaw-mosquitto"
	@echo "   sudo systemctl enable --now evoclaw-orchestrator"
	@echo "   sudo systemctl enable --now evoclaw-edge-agent"

install-systemd-bare: ## Install bare-metal agent systemd service (requires sudo)
	sudo cp deploy/systemd/evoclaw-agent-bare.service /etc/systemd/system/
	sudo systemctl daemon-reload
	@echo "✅ Bare-metal agent service installed. Enable with:"
	@echo "   sudo systemctl enable --now evoclaw-agent-bare"
