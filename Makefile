GO ?= go
BIN_DIR ?= bin
DIST_DIR ?= dist
VERSION_FILE ?= VERSION.txt
WORKSPACE_DIR ?= ./workspace
API_ADDR ?= 127.0.0.1:43180
SERVER_URL ?= http://$(API_ADDR)
CHAT_MSG ?= hello
SESSION ?=
PKG ?= ./...
TEST_NAME ?=
HEARTBEAT_INTERVAL ?= 30s
MAX_HEARTBEATS ?= 0
COVER_OUT ?= coverage.out
TARS_CONFIG ?= ./workspace/config/tars.config.yaml
ROOT_DIR := $(abspath .)
TARS_BIN := $(abspath $(BIN_DIR)/tars)
LAUNCH_AGENTS_DIR ?= $(HOME)/Library/LaunchAgents
LAUNCHCTL_DOMAIN ?= gui/$(shell id -u)
SERVER_LABEL ?= io.tars.server
ASSISTANT_LABEL ?= io.tars.assistant
SERVER_PLIST ?= $(LAUNCH_AGENTS_DIR)/$(SERVER_LABEL).plist
ASSISTANT_PLIST ?= $(LAUNCH_AGENTS_DIR)/$(ASSISTANT_LABEL).plist
SERVER_STDOUT_LOG ?= $(HOME)/Library/Logs/tars-server.out.log
SERVER_STDERR_LOG ?= $(HOME)/Library/Logs/tars-server.err.log
ASSISTANT_STDOUT_LOG ?= $(HOME)/Library/Logs/tars-assistant.out.log
ASSISTANT_STDERR_LOG ?= $(HOME)/Library/Logs/tars-assistant.err.log
LAUNCH_PATH ?= /opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin
ASSISTANT_SERVER_URL ?= $(SERVER_URL)
ASSISTANT_HOTKEY ?= Ctrl+Option+Space
ASSISTANT_AUDIO_INPUT ?= default
ASSISTANT_WHISPER_BIN ?= /opt/homebrew/bin/whisper-cli
ASSISTANT_OS_LOCALE ?= $(strip $(or $(LC_ALL),$(LANG),$(shell /usr/bin/defaults read -g AppleLocale 2>/dev/null)))
ASSISTANT_WHISPER_LANGUAGE ?= $(strip $(shell python3 -c 'import sys; locale=(sys.argv[1] if len(sys.argv) > 1 else "").strip().lower(); print("ko" if locale.startswith("ko") else "en" if locale.startswith("en") else "ja" if locale.startswith("ja") else "zh" if locale.startswith(("zh_cn","zh-hans","zh-chs","zh_tw","zh-hant","zh-cht")) else "auto")' '$(ASSISTANT_OS_LOCALE)'))
ASSISTANT_FFMPEG_BIN ?= /opt/homebrew/bin/ffmpeg
ASSISTANT_TTS_BIN ?= /usr/bin/say
ASSISTANT_API_TOKEN ?= $(TARS_API_TOKEN)
VERSION ?= $(strip $(shell test -f "$(VERSION_FILE)" && tr -d '[:space:]' < "$(VERSION_FILE)" || printf 'dev'))
GIT_COMMIT ?= $(strip $(shell git rev-parse --short HEAD 2>/dev/null || printf 'unknown'))
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILDINFO_PKG ?= github.com/devlikebear/tars/internal/buildinfo
comma := ,
CGO_LDFLAGS_EXTRA := $(if $(filter darwin,$(shell uname -s 2>/dev/null | tr A-Z a-z)),-Wl$(comma)-no_warn_duplicate_libraries,)
GO_LDFLAGS ?= -X $(BUILDINFO_PKG).Version=$(VERSION) -X $(BUILDINFO_PKG).Commit=$(GIT_COMMIT) -X $(BUILDINFO_PKG).Date=$(BUILD_DATE)
RELEASE_GOOS ?= $(shell $(GO) env GOOS)
RELEASE_GOARCH ?= $(shell $(GO) env GOARCH)
# Release archives use the no-CGO fallback path so darwin packaging does not
# depend on Objective-C hotkey bindings during CI cross-builds.
RELEASE_CGO_ENABLED ?= 0
RELEASE_ARCHIVE_NAME ?= tars_$(VERSION)_$(RELEASE_GOOS)_$(RELEASE_GOARCH).tar.gz
RELEASE_STAGE_DIR ?= $(DIST_DIR)/release-$(RELEASE_GOOS)-$(RELEASE_GOARCH)

.DEFAULT_GOAL := help

.PHONY: help \
	test test-v test-one test-nocache test-race test-cover \
	build build-bins release-asset clean tidy fmt vet lint \
	browser-install \
	install install-server install-assistant uninstall uninstall-server uninstall-assistant reinstall \
	restart restart-server restart-assistant reload-config reload-server-config reload-assistant-config \
	logs logs-server logs-server-err logs-assistant logs-assistant-err \
	dev-serve dev-serve-once dev-serve-loop dev-chat dev-heartbeat dev-tars \
	api-status api-sessions api-compact api-chat api-heartbeat smoke-auth \
	vault-up vault-down vault-logs security-scan \
	run-serve

help:
	@echo "Usage:"
	@echo "  make <target> [VAR=value]"
	@echo ""
	@echo "Common vars:"
	@echo "  PKG=./... TEST_NAME=TestRun_ChatMessage CHAT_MSG='hello'"
	@echo "  WORKSPACE_DIR=./workspace API_ADDR=127.0.0.1:43180 SERVER_URL=http://127.0.0.1:43180"
	@echo "  TARS_CONFIG=./config/standalone.yaml ASSISTANT_API_TOKEN=... LAUNCH_PATH=$(LAUNCH_PATH)"
	@echo "  ASSISTANT_WHISPER_LANGUAGE=$(ASSISTANT_WHISPER_LANGUAGE) (derived from locale; override with VAR=value)"
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
	@echo "  make release-asset - build a versioned release archive to $(DIST_DIR)"
	@echo "  make browser-install - npm install + playwright chromium install"
	@echo "  make install       - build $(TARS_BIN) and (re)install io.tars.server + io.tars.assistant launch agents"
	@echo "  make uninstall     - stop and remove io.tars.server + io.tars.assistant launch agents"
	@echo "  make reinstall     - uninstall then install launch agents"
	@echo "  make restart       - restart io.tars.server + io.tars.assistant without rebuilding"
	@echo "  make reload-config - reload current config by restarting launch agents"
	@echo "  make logs          - tail current server + assistant stdout logs"
	@echo "  make logs-server   - tail server stdout log"
	@echo "  make logs-server-err - tail server stderr log"
	@echo "  make logs-assistant - tail assistant stdout log"
	@echo "  make logs-assistant-err - tail assistant stderr log"
	@echo "  make fmt           - go fmt ./..."
	@echo "  make vet           - go vet ./..."
	@echo "  make lint          - alias of vet for quality checks"
	@echo "  make tidy          - go mod tidy"
	@echo "  make clean         - remove build artifacts"
	@echo ""
	@echo "Run targets:"
	@echo "  make dev-serve     - run server via tars serve (API mode)"
	@echo "  make dev-serve-once - run one heartbeat via tars serve"
	@echo "  make dev-serve-loop - run heartbeat loop via tars serve"
	@echo "  make dev-chat      - run Go client (cmd/tars)"
	@echo "  make dev-tars      - run Go client (cmd/tars)"
	@echo "  make dev-heartbeat - call heartbeat run-once via API"
	@echo ""
	@echo "API helpers:"
	@echo "  make api-status    - GET /v1/status"
	@echo "  make api-sessions  - GET /v1/sessions"
	@echo "  make api-compact   - POST /v1/compact"
	@echo "  make api-chat      - POST /v1/chat with CHAT_MSG"
	@echo "  make api-heartbeat - POST /v1/heartbeat/run-once"
	@echo "  make smoke-auth    - auth/role smoke test (requires USER_TOKEN, ADMIN_TOKEN)"
	@echo "  make security-scan - scan tracked files/history for secrets and local-path leaks"
	@echo ""
	@echo "Vault (docker compose):"
	@echo "  make vault-up      - start dev Vault + initializer"
	@echo "  make vault-logs    - follow vault-init logs"
	@echo "  make vault-down    - stop Vault stack"

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
	mkdir -p $(BIN_DIR)
	CGO_LDFLAGS="$(CGO_LDFLAGS_EXTRA)" $(GO) build -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/tars ./cmd/tars

build-bins:
	mkdir -p $(BIN_DIR)
	CGO_LDFLAGS="$(CGO_LDFLAGS_EXTRA)" $(GO) build -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/tars ./cmd/tars

release-asset:
	mkdir -p "$(DIST_DIR)"
	rm -rf "$(RELEASE_STAGE_DIR)"
	mkdir -p "$(RELEASE_STAGE_DIR)/share/tars"
	CGO_ENABLED=$(RELEASE_CGO_ENABLED) GOOS=$(RELEASE_GOOS) GOARCH=$(RELEASE_GOARCH) $(GO) build -ldflags "$(GO_LDFLAGS)" -o "$(RELEASE_STAGE_DIR)/tars" ./cmd/tars
	@if [ -d "./skills" ]; then cp -R "./skills" "$(RELEASE_STAGE_DIR)/share/tars/skills"; fi
	@if [ -d "./plugins" ]; then cp -R "./plugins" "$(RELEASE_STAGE_DIR)/share/tars/plugins"; fi
	tar -C "$(RELEASE_STAGE_DIR)" -czf "$(DIST_DIR)/$(RELEASE_ARCHIVE_NAME)" tars share

browser-install:
	npm install
	npx playwright install chromium

install: install-server install-assistant

install-server: build-bins
	@mkdir -p "$(LAUNCH_AGENTS_DIR)" "$(HOME)/Library/Logs"
	@{ \
		printf '%s\n' \
			'<?xml version="1.0" encoding="UTF-8"?>' \
			'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
			'<plist version="1.0">' \
			'<dict>' \
			'  <key>Label</key><string>$(SERVER_LABEL)</string>' \
			'  <key>ProgramArguments</key>' \
			'  <array>' \
			'    <string>$(TARS_BIN)</string>' \
			'    <string>serve</string>' \
			'    <string>--config</string>' \
			'    <string>$(abspath $(TARS_CONFIG))</string>' \
			'  </array>' \
			'  <key>WorkingDirectory</key><string>$(ROOT_DIR)</string>' \
			'  <key>RunAtLoad</key><true/>' \
			'  <key>KeepAlive</key><true/>' \
			'  <key>StandardOutPath</key><string>$(SERVER_STDOUT_LOG)</string>' \
			'  <key>StandardErrorPath</key><string>$(SERVER_STDERR_LOG)</string>' \
			'  <key>EnvironmentVariables</key>' \
			'  <dict>' \
			'    <key>PATH</key><string>$(LAUNCH_PATH)</string>' \
			'  </dict>' \
			'</dict>' \
			'</plist>'; \
	} > "$(SERVER_PLIST)"
	@launchctl bootout "$(LAUNCHCTL_DOMAIN)" "$(SERVER_PLIST)" >/dev/null 2>&1 || true
	@launchctl bootstrap "$(LAUNCHCTL_DOMAIN)" "$(SERVER_PLIST)"
	@launchctl kickstart -k "$(LAUNCHCTL_DOMAIN)/$(SERVER_LABEL)"

install-assistant: build-bins
	@mkdir -p "$(LAUNCH_AGENTS_DIR)" "$(HOME)/Library/Logs"
	@"$(TARS_BIN)" assistant install-launchagent \
		--server-url "$(ASSISTANT_SERVER_URL)" \
		--workspace-dir "$(abspath $(WORKSPACE_DIR))" \
		--hotkey "$(ASSISTANT_HOTKEY)" \
		--audio-input "$(ASSISTANT_AUDIO_INPUT)" \
		--whisper-bin "$(ASSISTANT_WHISPER_BIN)" \
		--whisper-language "$(ASSISTANT_WHISPER_LANGUAGE)" \
		--ffmpeg-bin "$(ASSISTANT_FFMPEG_BIN)" \
		--tts-bin "$(ASSISTANT_TTS_BIN)" \
		--label "$(ASSISTANT_LABEL)" \
		--plist-path "$(ASSISTANT_PLIST)" \
		--stdout-log "$(ASSISTANT_STDOUT_LOG)" \
		--stderr-log "$(ASSISTANT_STDERR_LOG)" \
		$(if $(ASSISTANT_API_TOKEN),--api-token "$(ASSISTANT_API_TOKEN)",) \
		--load

uninstall: uninstall-assistant uninstall-server

uninstall-server:
	@launchctl bootout "$(LAUNCHCTL_DOMAIN)" "$(SERVER_PLIST)" >/dev/null 2>&1 || true
	@rm -f "$(SERVER_PLIST)"

uninstall-assistant:
	@launchctl bootout "$(LAUNCHCTL_DOMAIN)" "$(ASSISTANT_PLIST)" >/dev/null 2>&1 || true
	@rm -f "$(ASSISTANT_PLIST)"

reinstall: uninstall install

restart: restart-server restart-assistant

restart-server:
	@launchctl kickstart -k "$(LAUNCHCTL_DOMAIN)/$(SERVER_LABEL)"

restart-assistant:
	@launchctl kickstart -k "$(LAUNCHCTL_DOMAIN)/$(ASSISTANT_LABEL)"

reload-config: reload-server-config reload-assistant-config

reload-server-config: restart-server

reload-assistant-config: restart-assistant

logs:
	@mkdir -p "$(HOME)/Library/Logs"
	@touch "$(SERVER_STDOUT_LOG)" "$(ASSISTANT_STDOUT_LOG)"
	@tail -f "$(SERVER_STDOUT_LOG)" "$(ASSISTANT_STDOUT_LOG)"

logs-server:
	@mkdir -p "$(HOME)/Library/Logs"
	@touch "$(SERVER_STDOUT_LOG)"
	@tail -f "$(SERVER_STDOUT_LOG)"

logs-server-err:
	@mkdir -p "$(HOME)/Library/Logs"
	@touch "$(SERVER_STDERR_LOG)"
	@tail -f "$(SERVER_STDERR_LOG)"

logs-assistant:
	@mkdir -p "$(HOME)/Library/Logs"
	@touch "$(ASSISTANT_STDOUT_LOG)"
	@tail -f "$(ASSISTANT_STDOUT_LOG)"

logs-assistant-err:
	@mkdir -p "$(HOME)/Library/Logs"
	@touch "$(ASSISTANT_STDERR_LOG)"
	@tail -f "$(ASSISTANT_STDERR_LOG)"

dev-serve:
	$(GO) run ./cmd/tars serve --verbose --serve-api $(if $(TARS_CONFIG),--config $(TARS_CONFIG),) --workspace-dir $(WORKSPACE_DIR) --api-addr $(API_ADDR) $(ARGS)

dev-serve-once:
	$(GO) run ./cmd/tars serve --verbose --run-once $(if $(TARS_CONFIG),--config $(TARS_CONFIG),) --workspace-dir $(WORKSPACE_DIR) $(ARGS)

dev-serve-loop:
	$(GO) run ./cmd/tars serve --verbose --run-loop $(if $(TARS_CONFIG),--config $(TARS_CONFIG),) --heartbeat-interval $(HEARTBEAT_INTERVAL) --max-heartbeats $(MAX_HEARTBEATS) --workspace-dir $(WORKSPACE_DIR) $(ARGS)

dev-chat:
	$(GO) run ./cmd/tars --server-url $(SERVER_URL) $(if $(SESSION),--session $(SESSION),) $(ARGS)

dev-tars:
	$(GO) run ./cmd/tars --server-url $(SERVER_URL) $(if $(SESSION),--session $(SESSION),) $(ARGS)

dev-heartbeat:
	curl -sS -X POST $(SERVER_URL)/v1/heartbeat/run-once

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

api-heartbeat:
	curl -sS -X POST $(SERVER_URL)/v1/heartbeat/run-once

smoke-auth:
	./scripts/smoke_auth_workspace.sh

security-scan:
	./scripts/security_scan.sh

vault-up:
	docker compose -f docker-compose.vault.yaml up -d

vault-logs:
	docker compose -f docker-compose.vault.yaml logs -f vault-init

vault-down:
	docker compose -f docker-compose.vault.yaml down

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint: vet

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR) $(COVER_OUT)

run-serve: dev-serve
