# tars-ui

React/TypeScript(INK) 기반 TARS TUI 클라이언트.

## Requirements

- Node.js 20+
- `tarsd` API 서버 실행 (`/v1/chat`, `/v1/sessions*`, `/v1/status`, `/v1/compact`, `/v1/heartbeat/run-once`)

## Quick start

```bash
cd tars-ui
npm install
npm run dev -- --server-url http://127.0.0.1:43180 --verbose
```

## Test

```bash
cd tars-ui
npm test
```

## Options

- `--server-url` : `tarsd` API URL (기본값: `http://127.0.0.1:43180`)
- `--session` : 시작 세션 ID
- `--verbose` : 우측 패널에 디버그 이벤트 표시

## Slash commands

- `/help`
- `/sessions`
- `/new [title]`
- `/resume {id}` 또는 `/resume` 후 번호 선택
- `/history`
- `/export`
- `/search {keyword}`
- `/status`
- `/compact`
- `/heartbeat`
- `/quit`, `/exit`
