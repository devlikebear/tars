# TARS Console 접속 방법

TARS Console은 로컬에서 실행되는 단일 Go 바이너리가 제공하는 웹 콘솔입니다. 같은 콘솔을 여러 경로로 접근할 수 있고, 사용 맥락에 따라 가장 잘 맞는 방법이 다릅니다.

| 방법 | 가장 잘 맞는 상황 |
|---|---|
| 브라우저 탭 | 가벼운 일회성 조회, 즉석 개발/디버깅 |
| PWA / Dock에 추가 | 매일 쓰는 데스크톱 앱처럼 띄우고 싶을 때 |
| CLI (`tars`) | 채팅 외 명령(`tars status`, `tars cron list` 등)이 필요할 때 |
| 백그라운드 서비스 | 로그인하면 자동으로 TARS를 시작하고 싶을 때 |

## 1. 브라우저 탭

```bash
tars serve
# → http://127.0.0.1:43180/console
```

`tars serve`가 켜져 있는 동안 어떤 브라우저에서든 위 주소로 접속할 수 있습니다. 가장 권장되는 일상 사용 방법은 이 콘솔을 PWA로 설치해서 데스크톱 앱처럼 띄우는 것입니다 (아래 2번).

## 2. PWA / Dock에 추가

콘솔은 PWA(Progressive Web App)로 설치할 수 있습니다. 설치하면 별도 창에서 실행되고, macOS Dock·Windows 작업표시줄·Linux 런처에 아이콘이 등록됩니다. 브라우저 UI(주소창·탭·확장 아이콘) 없이 콘솔만 보입니다.

### Chrome / Edge / Brave / Arc (모든 OS)

1. `http://127.0.0.1:43180/console`로 접속합니다.
2. 주소창 오른쪽의 **설치(Install) 아이콘**(모니터에 화살표가 있는 모양)을 클릭합니다.
   - 아이콘이 안 보이면 메뉴 → "TARS Console 설치" / "앱으로 설치"를 누릅니다.
3. 다이얼로그에서 **설치**를 눌러 확정합니다.

### Safari (macOS 14+)

1. Safari에서 `http://127.0.0.1:43180/console`로 접속합니다.
2. 메뉴 → **파일 → Dock에 추가** (또는 공유 시트 → "Dock에 추가").
3. 이름과 아이콘을 확인한 뒤 **추가**를 누릅니다.

> 참고: macOS 13 이하 Safari는 "Dock에 추가" 기능이 없습니다. 이 경우 Chrome 계열 브라우저로 설치하거나, AppleScript 단축어로 `http://127.0.0.1:43180/console`을 여는 방법을 사용하세요.

### Firefox

Firefox는 데스크톱 PWA 설치를 공식 지원하지 않습니다. 다른 브라우저로 설치하거나, 브라우저 탭에서 그대로 사용하세요.

### 설치 후 동작

- 별도 창으로 열리며 `start_url`은 `/console/`입니다.
- 백그라운드에서 `tars serve`가 실행 중이어야 합니다 (4번 참조).
- TARS 업그레이드로 콘솔 자산이 갱신되면, 앱을 한 번 새로고침하면 최신 콘솔이 적용됩니다.

## 3. CLI

콘솔에서 할 수 있는 거의 모든 일은 CLI로도 가능합니다:

```bash
tars status                # 서버/Pulse/Reflection 상태
tars health                # 상세 헬스체크
tars assistant "<message>" # 일회성 채팅
tars cron list             # 스케줄러 잡 조회
tars approve list          # 승인 대기 작업
tars skill list            # 설치된 스킬
```

자동화·스크립팅 용도라면 CLI가 가장 가볍습니다. 채팅 컨텍스트·세션 탐색·이벤트 스트림은 콘솔이 훨씬 편합니다.

## 4. 백그라운드 서비스 (LaunchAgent)

`tars`를 로그인 시 자동 시작하려면:

```bash
tars service install   # macOS LaunchAgent 설치
tars service start
tars service status
```

설치 후에는 콘솔(브라우저 탭이든 PWA든)에 접속하기만 하면 됩니다. 일상 사용 권장 조합은 **(2) PWA + (4) LaunchAgent**입니다.

## 트러블슈팅

- **PWA 설치 아이콘이 안 보임**: 매니페스트가 캐시됐을 수 있습니다. 강제 새로고침(Cmd/Ctrl+Shift+R) 후 다시 확인하세요.
- **별도 창으로 열리지 않고 새 탭으로 열림**: 브라우저가 PWA로 인식하지 못한 경우입니다. 설치 절차를 다시 진행하거나, 다른 Chromium 계열 브라우저를 사용해 보세요.
- **`Connection refused`**: `tars serve`가 실행 중인지 확인하세요. `tars status`로 점검할 수 있습니다.
- **원격 접속이 필요한 경우**: 로컬 콘솔은 기본적으로 `127.0.0.1`에만 바인딩됩니다. 원격 접근은 README의 *Remote Access* 섹션(Tailscale Serve)을 참고하세요.
