GO ?= go

.PHONY: help test build build-bins run-tarsd run-cased run-tars

help:
	@echo "Makefile commands:"
	@echo "  make test          - Run all tests"
	@echo "  make build         - Build the project"
	@echo "  make build-bins    - Build binaries for tarsd, cased, and tars"
	@echo "  make run-tarsd     - Run the tarsd command"
	@echo "  make run-cased     - Run the cased command"
	@echo "  make run-tars      - Run the tars command"

test:
	$(GO) test ./...

build:
	$(GO) build ./...

build-bins:
	mkdir -p bin
	$(GO) build -o bin/tarsd ./cmd/tarsd
	$(GO) build -o bin/cased ./cmd/cased
	$(GO) build -o bin/tars ./cmd/tars

run-tarsd:
	$(GO) run ./cmd/tarsd

run-cased:
	$(GO) run ./cmd/cased

run-tars:
	$(GO) run ./cmd/tars
