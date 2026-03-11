SHELL := /bin/bash

GO ?= $(shell \
	if command -v go1.24.0 >/dev/null 2>&1; then command -v go1.24.0; \
	elif [ -x "$$HOME/go/bin/go1.24.0" ]; then echo "$$HOME/go/bin/go1.24.0"; \
	else command -v go; fi)

APP := cursor2api-go
PORT ?= 8002
API_KEY ?= 0000

.PHONY: help deps node-deps test build run smoke upstream-check clean

help:
	@echo "Targets:"
	@echo "  make deps           # download Go deps"
	@echo "  make node-deps      # install Node runtime deps (OCR helper)"
	@echo "  make test           # run go test ./..."
	@echo "  make build          # build $(APP)"
	@echo "  make run            # run the server"
	@echo "  make smoke          # run live smoke checks"
	@echo "  make upstream-check # run live upstream matrix checks"
	@echo "  make clean          # remove built binary"

deps:
	$(GO) mod download

node-deps:
	@if [ -f package.json ]; then npm install --omit=dev; fi

test: deps
	$(GO) test ./...

build: deps node-deps
	$(GO) build -o $(APP) .

run:
	PORT=$(PORT) API_KEY=$(API_KEY) $(GO) run .

smoke:
	PORT=$(PORT) API_KEY=$(API_KEY) ./scripts/e2e_smoke.sh

upstream-check:
	PORT=$(PORT) API_KEY=$(API_KEY) ./scripts/e2e_upstream_matrix.sh

clean:
	rm -f $(APP) $(APP).exe
