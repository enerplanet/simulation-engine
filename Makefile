# Makefile for EnerplanET Simulation Engine
# Attention use !!! TABS !!! not !!! SPACES !!!

.PHONY: all help build build-nocache build-calliope build-webservice up down restart logs clean stop prune pv wind geothermal biomass

# Default target
all: help

#==============================================================================
# HELP
#==============================================================================
help:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════════╗"
	@echo "║        EnerplanET Simulation Engine - Makefile Commands          ║"
	@echo "╚══════════════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "BUILD COMMANDS:"
	@echo "  make build              Build all images (calliope + webservice)"
	@echo "  make build-nocache      Build all images without Docker cache"
	@echo "  make build-calliope     Build only the calliope base image"
	@echo "  make build-webservice   Build only the webservice image"
	@echo ""
	@echo "RUN COMMANDS:"
	@echo "  make up                 Start all simulation engine containers"
	@echo "  make up-min             Start in minimal mode"
	@echo "  make down               Stop all containers"
	@echo "  make restart            Restart all containers"
	@echo "  make logs               Show container logs (follow mode)"
	@echo ""
	@echo "INDIVIDUAL SERVICES:"
	@echo "  make pv                 Start photovoltaic simulation service"
	@echo "  make wind               Start wind onshore simulation service"
	@echo "  make geothermal         Start geothermal simulation service"
	@echo "  make biomass            Start biomass simulation service"
	@echo ""
	@echo "CLEANUP COMMANDS:"
	@echo "  make stop               Stop containers without removing images"
	@echo "  make clean              Stop and remove all containers and images"
	@echo "  make prune              Remove all unused Docker resources"
	@echo ""
	@echo "DEVELOPMENT:"
	@echo "  make dev                Start with hot reload (development mode)"
	@echo "  make generate-profiles  Generate SLP demand profiles"
	@echo ""

#==============================================================================
# BUILD COMMANDS
#==============================================================================
build: build-calliope build-webservice
	@echo "✓ All images built successfully"

build-nocache:
	@echo "Building calliope base image (no cache)..."
	docker build --network=host --no-cache --tag s6et_calliope:latest -f webservice.docker/calliope.dockerfile webservice.docker/ --platform linux/amd64
	@echo "Building webservice image (no cache)..."
	docker build --network=host --no-cache --tag s6et_webservice:latest -f webservice.docker/webservice.dockerfile webservice.docker/ --platform linux/amd64
	@echo "✓ All images built successfully (no cache)"

build-calliope:
	@echo "Building calliope base image..."
	docker build --network=host --tag s6et_calliope:latest -f webservice.docker/calliope.dockerfile webservice.docker/ --platform linux/amd64

build-webservice:
	@echo "Building webservice image..."
	docker build --network=host --tag s6et_webservice:latest -f webservice.docker/webservice.dockerfile webservice.docker/ --platform linux/amd64

#==============================================================================
# RUN COMMANDS
#==============================================================================
up:
	docker network create spatialhub-net 2>/dev/null || true
	docker volume create sim_shared_data 2>/dev/null || true
	docker compose -f docker-compose/docker-compose.unltd.yaml up -d
	@sleep 2
	docker network connect spatialhub-net sim-haproxy 2>/dev/null || true
	@echo "✓ Simulation engine started"
	@echo "  HAProxy Stats: http://localhost:8405/stats"
	@echo "  Health Check:  http://localhost:8089/health"

up-min:
	docker network create spatialhub-net 2>/dev/null || true
	docker volume create sim_shared_data 2>/dev/null || true
	docker compose -f docker-compose/docker-compose.unltd.dev.yaml up -d
	@sleep 2
	docker network connect spatialhub-net sim-haproxy 2>/dev/null || true
	@echo "✓ Simulation engine started"
	@echo "  HAProxy Stats: http://localhost:8405/stats"
	@echo "  Health Check:  http://localhost:8089/health"

down:
	docker compose -f docker-compose/docker-compose.unltd.yaml down

restart:
	docker compose -f docker-compose/docker-compose.unltd.yaml restart

logs:
	docker compose -f docker-compose/docker-compose.unltd.yaml logs -f

#==============================================================================
# INDIVIDUAL SERVICES
#==============================================================================
pv:
	docker compose -f docker-compose/docker-compose.photovoltaik.yaml up -d

wind:
	docker compose -f docker-compose/docker-compose.wind_onshore.yaml up -d

geothermal:
	docker compose -f docker-compose/docker-compose.geothermal.yaml up -d

biomass:
	docker compose -f docker-compose/docker-compose.biomass.yaml up -d

#==============================================================================
# CLEANUP COMMANDS
#==============================================================================
stop:
	@echo "Stopping running containers..."
	docker compose -f docker-compose/docker-compose.unltd.yaml down

clean:
	@echo "Stopping and cleaning up all Docker resources..."
	docker compose -f docker-compose/docker-compose.unltd.yaml down --rmi all

prune:
	@echo "Removing unused Docker images and volumes..."
	docker system prune -af
	docker volume prune -f

#==============================================================================
# DEVELOPMENT
#==============================================================================
dev:
	docker network create spatialhub-net 2>/dev/null || true
	docker compose -f docker-compose/docker-compose.dev.yaml up -d
	@echo "✓ Development mode started with hot reload"

generate-profiles:
	@echo "Generating SLP demand profiles..."
	python generate_slp_profiles.py
	@echo "✓ Profiles generated in webservice.docker/servicehub/data_new/"
