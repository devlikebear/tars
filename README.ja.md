# TARS (日本語)

TARS は 2 つの Go バイナリで構成された軽量ローカル AI 自動化スタックです。

- `tarsd`: デーモン/サーバー（LLM オーケストレーション、セッション、ツール、ゲートウェイ、自動化）
- `tars`: ターミナルクライアント（Bubble Tea 3 ペイン TUI）

現在の構成は、公開運用向けにシンプル化されています。

## 主な機能

- SSE ストリーミング Chat API（`/v1/chat`）
- セッション管理（`/v1/sessions`, history/export/search, compact）
- Agent loop + 組み込みツール（`read/write/edit/glob/exec/process/memory/cron/heartbeat/...`）
- In-process gateway runtime
  - 非同期 run（`/v1/agent/runs`）
  - channels/webhooks
  - browser/nodes/message ツール
- Skills/Plugins/MCP のホットリロード（`/v1/runtime/extensions/reload`）
- ブラウザ自動化ランタイム
  - status/profiles/login/check/run API
  - ローカル browser relay（`/extension`, `/cdp`）+ token/origin/loopback 検証
- Vault read-only 連携（オプション）：ブラウザ自動ログインフロー

## リポジトリ構成

- `cmd/tarsd`: メインサーバー
- `cmd/tars`: Go TUI クライアント
- `internal/*`: ランタイムモジュール（gateway, tool, llm, session, extensions, browser, vaultclient, ...）
- `config/tarsd.config.example.yaml`: 設定例
- `workspace/`: 実行用ワークスペース（sessions, memory, automation など）

## クイックスタート

### 1) 前提

- Go 1.24+
- LLM provider credential（例：`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`）

### 2) 設定

デフォルト設定ファイル:

- `workspace/config/tarsd.config.yaml`

または設定例から開始:

- `config/tarsd.config.example.yaml`

### 3) サーバー起動

```bash
make dev-tarsd
```

デフォルト API アドレス:

- `http://127.0.0.1:43180`

### 4) クライアント起動

```bash
make dev-tars
```

### 5) スモークチェック

```bash
make api-status
make api-sessions
make smoke-auth
```

## 認証 / 認可

`api_auth_mode` はロール別トークンをサポートします。

- `api_user_token`: チャット/一般操作
- `api_admin_token`: 制御操作（`/v1/runtime/extensions/reload`, `/v1/gateway/reload`, `/v1/gateway/restart`, channel inbound）

ワークスペーススコープヘッダー:

- `Tars-Workspace-Id`

## cmd/tars の主なコマンド

- チャット + status trace パネル
- セッション: `/new`, `/sessions`, `/resume`, `/history`, `/export`, `/search`
- ランタイム: `/agents`, `/spawn`, `/runs`, `/run`, `/cancel-run`, `/gateway`, `/channels`
- 自動化: `/cron`, `/notify`, `/heartbeat`
- Browser/Vault:
  - `/browser status|profiles|login|check|run`
  - `/vault status`

## Browser + Vault（オプション）

`tarsd` 設定で有効化:

- `vault_enabled: true`
- `browser_runtime_enabled: true`
- `browser_relay_enabled: true`
- `tools_browser_enabled: true`

任意の site flow ディレクトリ:

- `browser_site_flows_dir: ./workspace/automation/sites`

`vault_form` ログインモードでは allowlist 設定が必要です:

- `vault_secret_path_allowlist_json`
- `browser_auto_login_site_allowlist_json`

## Docker Compose で Vault（dev）

```bash
docker compose -f docker-compose.vault.yaml up -d
docker compose -f docker-compose.vault.yaml logs -f vault-init
```

セットアップ内容:

- Vault dev server: `http://127.0.0.1:8200`
- KV v2 mount: `tars`
- sample secret: `tars/sites/grafana`（`username`, `password`）
- readonly policy: `tars-readonly`
- readonly token: `vault-init` ログに出力

停止:

```bash
docker compose -f docker-compose.vault.yaml down
```

## テスト

```bash
make test
# または
go test ./... -count=1
```

## セキュリティスキャン

```bash
make security-scan
```

実行内容:

- `gitleaks` 履歴スキャン
- 絶対ローカルパス漏えいチェック（`/Users/...`）
- private key marker チェック

## 補足

- `cased` sentinel デーモンは簡素化のため削除済み
- 本番でのプロセス監視は systemd/launchd/docker に委譲
- `GET /v1/healthz` は外部ヘルスプローブ用に維持

## コントリビュート

バージョニング/PR ルールは [CONTRIBUTING.md](CONTRIBUTING.md) を参照してください。
