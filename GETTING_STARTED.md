# TARS Getting Started

이 문서는 `tars`를 실무 자동화에 바로 쓰기 위한 최소 가이드입니다.

## 1) 3분 시작

1. 서버 실행

```bash
make dev-serve
```

2. 클라이언트 실행

```bash
make dev-tars
```

3. 동작 확인 (`tars` TUI 내부)

```text
/health
/whoami
/status
```

## 2) 실무형 사용 예시 (자연어 프롬프트)

아래는 그대로 입력해서 시작할 수 있는 예시입니다.

1. 웹 헬스체크 + 다중 채널 리포트

```text
매 10분마다 grafana 로그인 가능 여부를 점검하고, 실패하면 webhook 채널과 telegram 채널로 동시에 보고하는 크론잡을 등록해줘.
```

2. 뉴스 수집 + 뉴스레터 발송

```text
매일 오전 8시에 AI/클라우드 뉴스를 수집해서 10줄 요약 뉴스레터를 만들고 channels의 newsletter 채널로 보내는 작업을 등록해줘.
```

3. 연재형 소설 자동 집필

```text
매일 밤 10시에 SF 소설 1,200자 분량을 이어서 작성하고 workspace/novel/chapter-log.md에 누적 저장하도록 크론잡 만들어줘.
```

4. 주기적 개발 작업 실행

```text
평일 오전 9시에 현재 TODO를 읽고 가장 작은 개발 작업 1개를 구현하고 테스트까지 수행한 뒤 결과를 daily log에 남기도록 예약해줘.
```

5. 운영 점검 루틴

```text
매시 정각마다 시스템 상태(디스크, 주요 프로세스, 최근 오류 로그)를 점검하고 이상 시 운영 채널로 보고하는 점검 작업을 등록해줘.
```

## 3) 운영자가 자주 쓰는 명령

```text
/cron list
/cron runs <job_id>
/notify list
/runs
/run <run_id>
/gateway status
```

## 4) 바로 적용 팁

- 브라우저 로그인 자동화가 필요하면 site flow + Vault를 먼저 설정하세요.
- 보고 채널이 여러 개라면 webhook/telegram 채널을 먼저 등록해두면 프롬프트만으로 조합 가능합니다.
- 처음에는 `5~10분` 주기로 테스트하고, 검증 후 주기를 늘리는 것이 안전합니다.

## 5) 텔레그램 연동 테스트

### 5-1) 자동 페어링(Polling, 권장)

`chat_id`를 미리 입력하지 않고, 봇 토큰만으로 페어링할 수 있습니다.

1. 서버 설정

- `workspace/config/tars.config.yaml` 확인
  - `channels_telegram_enabled: true`
  - `channels_telegram_dm_policy: pairing`
  - `channels_telegram_polling_enabled: true`
- `.env.secret`(또는 `.env`) 설정

```bash
TELEGRAM_BOT_TOKEN=<YOUR_BOT_TOKEN>
TARS_API_TOKEN=1234
TARS_ADMIN_API_TOKEN=admin
```

2. 서버 실행

```bash
make dev-serve
```

3. 사용자 페어링 요청

- Telegram에서 봇에게 아무 메시지(예: `hello`)를 보냅니다.
- 봇이 `Pairing code: XXXXXXXX` 메시지를 회신합니다.

4. 관리자 승인

- tars 클라이언트에서 승인:

```bash
tars /telegram pairing approve XXXXXXXX
```

- 또는 Admin API 직접 호출:

```bash
curl -sS -X POST "http://127.0.0.1:43180/v1/channels/telegram/pairings/approve" \
  -H "Authorization: Bearer admin" \
  -H "Content-Type: application/json" \
  -d '{"code":"XXXXXXXX"}'
```

5. 검증

- 승인 후 같은 사용자가 보낸 다음 메시지는 LLM 응답으로 회신됩니다.
- `tars /telegram pairings`에서 pending/allowed 상태를 확인할 수 있습니다.

### 5-2) Outbound API 단독 테스트(수동 chat_id)

기존 방식대로 `chat_id`를 알고 있을 때는 아래 API로 즉시 전송할 수 있습니다.

```bash
curl -sS -X POST "http://127.0.0.1:43180/v1/channels/telegram/send" \
  -H "Authorization: Bearer 1234" \
  -H "Content-Type: application/json" \
  -d '{
    "chat_id": "<CHAT_ID>",
    "text": "tars telegram integration test"
  }'
```

- Telegram 채팅방에 메시지가 도착해야 합니다.
- API 응답 JSON에 `source: "telegram"` 및 `direction: "outbound"`가 포함되어야 합니다.

### 5-3) Telegram 슬래시 명령 / typing / 미디어 인바운드

1. 지원 명령(텔레그램 DM에서 `/`로 실행)

- `/help`
- `/sessions`
- `/status`
- `/health`
- `/cron list`
- `/cron runs {job_id} [limit]`
- `/gateway status`
- `/channels`

2. 세션 정책

- `session_telegram_scope: main`(기본값): Telegram은 메인 세션을 공유합니다. `/new`, `/resume`은 차단됩니다.
- `session_telegram_scope: per-user`: 사용자별 세션을 사용하며 `/new`, `/resume`이 허용됩니다.

3. typing 이벤트

- 일반 텍스트/LLM 경로에서만 Telegram `typing` 이벤트를 주기적으로 전송합니다.
- 명령 경로(`/help` 등)는 typing 없이 즉시 응답합니다.

4. 미디어 인바운드(photo/document/voice)

- private chat에서 `photo`, `document`, `voice`를 수신/저장합니다.
- 저장 경로: `workspace/telegram/media/<YYYYMMDD>/chat_<chat_id>/...`
- 최대 크기: `20MB` (`telegramMediaMaxBytes`).
- 캡션(또는 텍스트) 포함:
  - 첨부 메타(`saved_path`, `mime`, `size`, `original_name`)를 user prompt에 주입해 LLM이 응답합니다.
- 캡션 없음:
  - 파일만 저장하고, "캡션/텍스트를 추가로 보내달라"는 안내를 반환합니다(LLM 미호출).
