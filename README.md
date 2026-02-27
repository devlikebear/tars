# TARS (한국어)

TARS는 단일 Go 바이너리 기반의 경량 로컬 AI 자동화 스택입니다.

- `tars`: 터미널 클라이언트 (Bubble Tea 3패널 TUI) + 서버 모드 (`tars serve`)

현재 구조는 공개 사용/운영 단순화를 목표로 정리되어 있습니다.

## 주요 기능

- SSE 스트리밍 채팅 API (`/v1/chat`)
- 세션 수명주기 (`/v1/sessions`, history/export/search, compact)
- Agent loop + 빌트인 도구 (`read/write/edit/glob/exec/process/memory/cron/heartbeat/...`)
- In-process gateway runtime
  - 비동기 run (`/v1/agent/runs`)
  - channels/webhooks
  - browser/nodes/message 도구
- 스킬/플러그인/MCP 핫리로드 (`/v1/runtime/extensions/reload`)
- 브라우저 자동화 런타임
  - status/profiles/login/check/run API
  - 로컬 브라우저 릴레이 (`/extension`, `/cdp`) + token/origin/loopback 검증
- Vault read-only 연동(옵션): 브라우저 자동 로그인 워크플로우

## 저장소 구조

- `cmd/tars`: 단일 바이너리 (`tars` TUI 클라이언트 + `tars serve` 서버 모드)
- `internal/*`: 런타임 모듈 (gateway, tool, llm, session, extensions, browser, vaultclient, ...)
- `config/tars.config.example.yaml`: 예시 설정
- `workspace/`: 런타임 워크스페이스 (sessions, memory, automation 등)

## 빠른 시작

### 1) 요구사항

- Go 1.24+
- LLM provider credential (예: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`)

### 2) 설정

기본 로컬 설정 파일:

- `config/standalone.yaml`

또는 예시 파일에서 시작:

- `config/tars.config.example.yaml`

### 3) 서버 실행

```bash
make dev-serve
```

기본 API 주소:

- `http://127.0.0.1:43180`

### 4) 클라이언트 실행

```bash
make dev-tars
```

### 5) 스모크 체크

```bash
make api-status
make api-sessions
make smoke-auth
```

## 인증 / 권한

`api_auth_mode`는 role 기반 token을 지원합니다.

- `api_user_token`: 채팅/일반 작업
- `api_admin_token`: 제어 작업 (`/v1/runtime/extensions/reload`, `/v1/gateway/reload`, `/v1/gateway/restart`, channel inbound)

## cmd/tars 핵심 명령

- 채팅 + status trace 패널
- 세션: `/new`, `/sessions`, `/resume`, `/history`, `/export`, `/search`
- 런타임: `/agents`, `/spawn`, `/runs`, `/run`, `/cancel-run`, `/gateway`, `/channels`
- 자동화: `/cron`, `/notify`, `/heartbeat`
- 브라우저/Vault:
  - `/browser status|profiles|login|check|run`
  - `/vault status`

## Browser + Vault (옵션)

`tars` 설정에서 활성화:

- `vault_enabled: true`
- `browser_runtime_enabled: true`
- `browser_relay_enabled: true`
- `tools_browser_enabled: true`

옵션 site flow 디렉터리:

- `browser_site_flows_dir: ./workspace/automation/sites`

`vault_form` 로그인 모드에서는 allowlist를 반드시 설정하세요:

- `vault_secret_path_allowlist_json`
- `browser_auto_login_site_allowlist_json`

## Docker Compose로 Vault(dev)

```bash
docker compose -f docker-compose.vault.yaml up -d
docker compose -f docker-compose.vault.yaml logs -f vault-init
```

구성:

- Vault dev server: `http://127.0.0.1:8200`
- KV v2 mount: `tars`
- sample secret: `tars/sites/grafana` (`username`, `password`)
- readonly policy: `tars-readonly`
- readonly token: `vault-init` 로그에 출력

종료:

```bash
docker compose -f docker-compose.vault.yaml down
```

## 테스트

```bash
make test
# 또는
go test ./... -count=1
```

## 보안 스캔

```bash
make security-scan
```

실행 항목:

- `gitleaks` 히스토리 스캔
- 절대 로컬 경로 노출 검사 (`/Users/...`)
- private key marker 검사

## 참고

- `cased` sentinel 데몬은 간소화 과정에서 제거됨
- 운영 환경 프로세스 감시는 systemd/launchd/docker로 위임
- `GET /v1/healthz`는 외부 health probe 용도로 유지

## 기여

기여 정책(버전관리/PR 기준)은 [CONTRIBUTING.md](CONTRIBUTING.md)를 참고하세요.
