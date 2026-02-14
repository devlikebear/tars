GO ?= go
BIN_DIR ?= bin
WORKSPACE_DIR ?= ./workspace
API_ADDR ?= 127.0.0.1:8080
SERVER_URL ?= http://$(API_ADDR)
CHAT_MSG ?= hello
SESSION ?=
PKG ?= ./...
TEST_NAME ?=
HEARTBEAT_INTERVAL ?= 30s
MAX_HEARTBEATS ?= 0
COVER_OUT ?= coverage.out

.DEFAULT_GOAL := help

.PHONY: help \
	test test-v test-one test-nocache test-race test-cover \
	build build-bins clean tidy fmt vet \
	dev-tarsd dev-tarsd-once dev-tarsd-loop dev-cased dev-tars dev-chat dev-heartbeat \
	api-status api-sessions api-compact api-chat \
	run-tarsd run-cased run-tars

help:
	@echo "Usage:"
	@echo "  make <target> [VAR=value]"
	@echo ""
	@echo "Common vars:"
	@echo "  PKG=./... TEST_NAME=TestRun_ChatMessage CHAT_MSG='hello'"
	@echo "  WORKSPACE_DIR=./workspace API_ADDR=127.0.0.1:8080 SERVER_URL=http://127.0.0.1:8080"
	@echo ""
	@echo "Test targets:"
	@echo "  make test          - go test $(PKG)"
	@echo "  make test-v        - verbose tests"
	@echo "  make test-one      - single test by TEST_NAME in PKG"
	@echo "  make test-nocache  - disable go test cache"
	@echo "  make test-race     - run race detector"
	@echo "  make test-cover    - write coverage to $(COVER_OUT)"
	@echo ""
	@echo "Build/quality targets:"
	@echo "  make build         - go build ./..."
	@echo "  make build-bins    - build cmd binaries to $(BIN_DIR)"
	@echo "  make fmt           - go fmt ./..."
	@echo "  make vet           - go vet ./..."
	@echo "  make tidy          - go mod tidy"
	@echo "  make clean         - remove build artifacts"
	@echo ""
	@echo "Run targets:"
	@echo "  make dev-tarsd     - run tarsd API server in verbose mode"
	@echo "  make dev-tarsd-once - run one heartbeat on tarsd"
	@echo "  make dev-tarsd-loop - run heartbeat loop on tarsd"
	@echo "  make dev-cased     - run cased in verbose mode"
	@echo "  make dev-tars      - run tars chat once (CHAT_MSG)"
	@echo "  make dev-chat      - run tars chat with optional SESSION"
	@echo "  make dev-heartbeat - call heartbeat run-once via tars client"
	@echo ""
	@echo "API helpers:"
	@echo "  make api-status    - GET /v1/status"
	@echo "  make api-sessions  - GET /v1/sessions"
	@echo "  make api-compact   - POST /v1/compact"
	@echo "  make api-chat      - POST /v1/chat with CHAT_MSG"

test:
	$(GO) test $(PKG)

test-v:
	$(GO) test -v $(PKG)

test-one:
	$(GO) test -v -run "$(TEST_NAME)" $(PKG)

test-nocache:
	$(GO) test -count=1 $(PKG)

test-race:
	$(GO) test -race $(PKG)

test-cover:
	$(GO) test -coverprofile=$(COVER_OUT) $(PKG)

build:
	$(GO) build ./...

build-bins:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/tarsd ./cmd/tarsd
	$(GO) build -o $(BIN_DIR)/cased ./cmd/cased
	$(GO) build -o $(BIN_DIR)/tars ./cmd/tars

dev-tarsd:
	$(GO) run ./cmd/tarsd --verbose --serve-api --workspace-dir $(WORKSPACE_DIR) --api-addr $(API_ADDR) $(ARGS)

dev-tarsd-once:
	$(GO) run ./cmd/tarsd --verbose --run-once --workspace-dir $(WORKSPACE_DIR) $(ARGS)

dev-tarsd-loop:
	$(GO) run ./cmd/tarsd --verbose --run-loop --heartbeat-interval $(HEARTBEAT_INTERVAL) --max-heartbeats $(MAX_HEARTBEATS) --workspace-dir $(WORKSPACE_DIR) $(ARGS)

dev-cased:
	$(GO) run ./cmd/cased --verbose $(ARGS)

dev-tars:
	$(GO) run ./cmd/tars --verbose chat -m "$(CHAT_MSG)" --server-url $(SERVER_URL) $(ARGS)

dev-chat:
	$(GO) run ./cmd/tars --verbose chat -m "$(CHAT_MSG)" --server-url $(SERVER_URL) $(if $(SESSION),--session $(SESSION),) $(ARGS)

dev-heartbeat:
	$(GO) run ./cmd/tars --verbose heartbeat run-once --server-url $(SERVER_URL) $(ARGS)

api-status:
	curl -sS $(SERVER_URL)/v1/status

api-sessions:
	curl -sS $(SERVER_URL)/v1/sessions

api-compact:
	curl -sS -X POST $(SERVER_URL)/v1/compact

api-chat:
	curl -sS -N -X POST $(SERVER_URL)/v1/chat \
		-H "Content-Type: application/json" \
		-d "{\"message\":\"$(CHAT_MSG)\"}"

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR) $(COVER_OUT)

run-tarsd: dev-tarsd

run-cased: dev-cased

run-tars: dev-tars
