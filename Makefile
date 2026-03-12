SHELL := /bin/bash

GO ?= $(shell \
	if command -v go1.24.0 >/dev/null 2>&1; then command -v go1.24.0; \
	elif [ -x "$$HOME/go/bin/go1.24.0" ]; then echo "$$HOME/go/bin/go1.24.0"; \
	else command -v go; fi)

APP := cursor2api-go
PORT ?= 8002
API_KEY ?= 0000
VISION_LANGUAGES ?= eng,chi_sim

.PHONY: help deps test build run self-check smoke upstream-check clean

help:
	@echo "Targets:"
	@echo "  make deps           # download Go deps"
	@echo "  make test           # run go test ./..."
	@echo "  make build          # build $(APP)"
	@echo "  make run            # run the server"
	@echo "  make self-check     # run local OCR/backend self-check"
	@echo "  make smoke          # run live smoke checks"
	@echo "  make upstream-check # run live upstream matrix checks"
	@echo "  make clean          # remove built binary"

deps:
	$(GO) mod download

test: deps
	$(GO) test ./...

build: deps
	$(GO) build -o $(APP) .

run:
	PORT=$(PORT) API_KEY=$(API_KEY) $(GO) run .

self-check:
	VISION_LANGUAGES=$(VISION_LANGUAGES) GO_BIN=$(GO) ./scripts/local_self_check.sh

smoke:
	PORT=$(PORT) API_KEY=$(API_KEY) ./scripts/e2e_smoke.sh

upstream-check:
	PORT=$(PORT) API_KEY=$(API_KEY) ./scripts/e2e_upstream_matrix.sh

clean:
	rm -f $(APP) $(APP).exe
