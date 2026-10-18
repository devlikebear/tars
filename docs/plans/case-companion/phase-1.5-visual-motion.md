# Phase 1.5 — CASE 봇 리디자인 (codex 래스터 스프라이트)

> Roadmap: [roadmap.md](./roadmap.md) · 선행: Phase 1 (#1265, 머지됨 — 이 페이즈가 그 미어캣을 대체한다)
> 목표: CASE를 **TARS의 짝인 동반 로봇**으로 다시 그린다. 몸체는 codex CLI 이미지 생성(`image_generation`, codex-cli 0.144.3에서 stable·켜짐)으로 그린 **래스터 스프라이트**, 표정·모션은 CSS.
> 표정 결정 로직(`companionExpression`, cue 머신, 서버 cue)은 건드리지 않는다. 바뀌는 건 몸을 그리는 방식과 모션이다.

## 0. 캐릭터를 미어캣에서 봇으로 바꾼 이유 (2026-10-18)

미어캣(A안)은 Focus Crew의 동물 크루와 같은 세계관으로 고른 캐릭터였다. Crew는 2026-10-17에 철회됐으므로 그 세계관을 따를 이유가 없다. TARS라는 이름(README: *Interstellar*의 TARS에서 빌림, 영화와 무관)의 짝인 CASE는 원래 시스템 성격 그대로 **로봇**이어야 맞다. 미어캣 이전의 CASE(#1201~#1248)도 안테나 달린 작은 봇 얼굴이었다.

캐릭터 지정 (V0 프롬프트의 기준):
- **몸체**: 모서리가 둥근 작은 직사각형 모놀리스(높이 ≈ 폭×1.4), 가운데 이음매로 나뉜 **두 개의 관절 판**, 짙은 그래파이트 금속(`#2B2E2C`~`#3A3E3B`)에 밝은 베벨 — 콘솔의 Graphite Signal 톤 그대로.
- **얼굴**: 몸체 위쪽 1/3의 어두운 유리 **화면**. 눈은 시그널 그린(`#3ee07f`, 콘솔 `--primary`) 둥근 사각형 두 개. 입은 없다(말할 때만 화면에 파형).
- **상태 LED**: 짧은 안테나 끝의 동그란 LED — 미어캣 이전 CASE가 안테나 끝에 상태색을 싣던 규칙으로 돌아간다.
- **팔·발**: 옆에 경첩으로 달린 얇은 판 모양 팔 두 개, 짧은 발 두 개.
- **성격**: 차분하고 믿음직하며 건조한 위트. 귀엽되 유치하지 않게.
- **독창성**: 영화·게임·브랜드의 로봇을 복제하지 않는다. "관절 판으로 된 모놀리스"라는 분위기만 빌리고, 화면 얼굴·안테나·작은 비율로 다른 캐릭터로 만든다. 프롬프트에도 "not a replica of any robot from a film, game or existing brand"를 넣는다.

## 1. 현재 상태 검토

잘 되어 있는 것 (유지):
- 표정 8종을 모양으로 구분하고, 상태색은 한 곳(지금은 가슴 칩)에만 둔다. `companionExpression`은 순수 함수이고 테스트가 있다.
- cue 머신(warning/justFinished/justArrived/longQuiet/server), 동작 5종, idle play, 시선 추적, 숨은 탭·reduced-motion 정지 조건, 히트 영역 고정. 계약이 문서와 테스트로 묶여 있다.
- e2e 6개(`companion-*.spec.ts`)와 `tests/companionPet.test.ts`가 표정·모션 규칙을 지킨다.

한계:
- **캐릭터가 시스템과 안 맞는다** (§0).
- **그림의 상한**: 64×64 격자 위 span 도형으로는 금속 질감·베벨·화면 발광이 안 된다.
- **모션의 상한**: 표정이 즉시 바뀐다(전환 없음). 등장·퇴장 연출이 없다.
- **말하는 상태가 없다**: CASE가 답을 스트리밍하는 동안에도 얼굴은 `working`뿐이다.
- `CompanionPet.svelte`가 1,707줄 — 말풍선·질문·몸·모션이 한 파일에 있다. 몸을 분리해야 이후 Bridge(P2) 작업과 파일이 덜 겹친다.

## 2. 렌더링 방식

### 2.1 래스터 스프라이트 (2026-10-18 사용자 결정)

바뀌는 규칙 — `DESIGN.md` "CASE's expressions", `HANDOFF.md` §2, `roadmap.md` 결정 1·3, `tests/companionPet.test.ts:429`(`<img|<svg` 금지 검사)를 함께 고친다:

- 허용: `frontend/console/src/assets/case/`의 **스프라이트 시트**(WebP, 알파). §5의 파이프라인으로 만든 파일만. CSS `background-image` + `background-position`(`steps()`)으로 재생.
- 계속 금지: 그 폴더 밖 이미지, 외부 URL, SVG 몸체, 폰트 아이콘, Lottie·GIF·APNG 같은 애니메이션 포맷/라이브러리, JS 프레임 루프.
- 그대로: 상태색은 한 곳에만(이제 **안테나 LED**), 이웃 표정과 두 군데 이상 모양 차이, reduced-motion 셀렉터 개별 나열, 히트 영역 고정, 숨은 탭에서 타이머·리스너 정지.

### 2.2 권장: 몸체는 래스터, 화면 속 얼굴은 CSS

봇은 얼굴이 **발광하는 화면**이다. 화면 위의 눈·눈썹·파형은 원래 단순한 빛 도형이라 CSS로 그리는 편이 더 선명하고, 래스터 방식의 약점 대부분이 사라진다.

| | 얼굴도 래스터 프레임 | **몸체 래스터 + 화면 얼굴 CSS (권장)** |
|---|---|---|
| 생성할 장수 | 몸 포즈 × 표정 × 깜빡임·시선 ≈ 30장 | 몸 포즈만 ≈ 10장 |
| 프레임 간 일관성 | 얼굴마다 편집·합성 필요 | 얼굴은 코드라 항상 같음 |
| 시선 추적 | 좌/정면/우 3프레임으로 축소 | 지금처럼 연속 3px (`companionGazeOffset` 유지) |
| 표정 전환 | 레이어 교차 페이드 | 눈 모양이 연속으로 변형 (`border-radius`·`scale`) |
| 64px 가독성 | 축소하면 발광 눈이 번짐 | 화면 해상도와 무관하게 선명 |
| 말하기 | 입 프레임 | 화면 하단 파형 막대 (CSS) |

권장안에서도 몸체(금속·베벨·팔·포즈)는 전부 codex 래스터라 "예쁘게 그린" 결과물은 그대로다. CSS는 화면 사각형 안에서만 그린다. 화면 위치는 포즈마다 다르므로 manifest에 프레임별 `screen: {x, y, w, h}`를 기록하고, CSS 얼굴은 그 값(custom properties)에 맞춰 놓인다. 얼굴까지 래스터로 할지는 V0에서 스타일과 함께 정한다.

### 2.3 래스터의 약점과 대응

| 약점 | 대응 |
|---|---|
| 생성할 때마다 캐릭터가 조금씩 달라진다 | 마스터 1장을 고정하고, 나머지 포즈는 `codex exec -i master.png -- "…"` 참조 편집으로 만든다. 편집은 픽셀을 보존하지 않으므로(V0 확인) **바뀌어야 할 영역(팔·안테나)만 마스크로 잘라 마스터 위에 합성** — 나머지 몸체는 모든 프레임에서 픽셀 단위로 같다 |
| 상태색이 그림에 구워진다 | 스프라이트의 LED는 **밝은 회색으로 비워 둔 채** 그리고, 상태색 LED는 CSS 요소(`.companion-gem`, 이름 유지)로 안테나 끝에 얹는다 → `--case-gem-*` 규칙과 테스트 유지 |
| 용량·해상도 | 셀 128×128(2x) 한 벌. 전체 상한 **150KB**(테스트로 검사). base 시트만 즉시 로드, 동작 시트는 마운트 후 `decode()`로 미리 디코드 |
| "모양으로 구분"을 코드로 못 지킨다 | 권장안이면 얼굴은 CSS라 지금처럼 코드로 지킨다. 얼굴 래스터안이면 e2e 갤러리 비교를 LED를 가리고 돌린다 |
| 출처 | `design/v2/README.md`에 AI 생성 에셋이라는 사실, 프롬프트, 생성일 기록 |

## 3. 표정 (봇)

`companionExpression`이 고르는 8종은 그대로, 모양만 봇에 맞게 바꾼다. 화면 얼굴(눈·눈썹·파형)은 CSS, 몸 포즈는 래스터 프레임.

| 표정 | 화면 속 눈 | 화면 속 눈썹 | 몸 포즈 (래스터) | LED |
|---|---|---|---|---|
| `neutral` | 둥근 사각형, 깜빡임 | 없음 | 기본 | 회색 |
| `working` | 가로로 납작해져 아래를 봄 + 아래에 진행 점 3개 | 없음 | 두 팔을 앞으로 (작업 중) | 앰버, 맥동 |
| `alert` | 크게, 깜빡임 멈춤 | 둘 다 올라간 선 | 안테나 쭉 뻗음, 팔 살짝 벌림 | 주황 |
| `happy` | ∩ 아치 | 없음 | 기본 | 녹색 |
| `upset` | 작은 점 | 가운데로 처진 선 | 어깨(판) 처짐 | 주황 |
| `wary` | 한쪽 가늘게 + 곁눈 | 한쪽 올림·한쪽 내림 | 기본 | 앰버 |
| `sleepy` | 가는 선, 화면 어두워짐 | 없음 | 살짝 기울어 내려앉음, `z` | 어두운 회색 |
| `greeting` | 윙크 (아치 하나 + 뜬 눈 하나) | 없음 | 한쪽 팔 들기 | 녹색 |

`alert`와 `upset`은 LED 색이 같으므로 눈 크기·눈썹 방향·포즈 셋이 다르다 (DESIGN.md 규칙). 눈은 항상 시그널 그린 한 가지 — 상태에 따라 눈 색을 바꾸지 않는다(그러면 "색만 다른 표정"이 된다).

## 4. 프레임 목록 (권장안 기준 약 10장)

| 시트 | 프레임 | 용도 |
|---|---|---|
| `base` | 기본 서기 (화면 비움) | neutral·happy·wary 등 대부분 |
| `pose` | 작업(팔 앞으로), 경계(안테나 뻗음·팔 벌림), 처짐(upset), 내려앉음(sleepy) × 1 | 표정별 포즈 |
| `wave` | 한쪽 팔 들기 3컷 | greeting 동작 |
| `split` | 두 판이 살짝 벌어졌다 돌아오는 2컷 | 머지 축하(F3) |
| `off` | 화면 꺼지고 몸을 낮춘 절전 자세 1컷 | 숨기기(F2)·부팅 등장 |
| (CSS만) | — | 화면 얼굴 전부, bounce·shake·tilt, 호흡, 떠오름, 부팅 스캔라인, z |

얼굴도 래스터로 정하면 `face` 시트(8표정 + 깜빡임 3 + 시선 좌우 2 + 말하기 2 ≈ 15장)가 더해진다.

## 5. 생성·후처리 파이프라인

```
docs/plans/case-companion/design/v2/
  prompts.md          프레임별 프롬프트 (재생성 가능하게, 그대로 커밋)
  source/*.png        codex 원본 (512px로 줄여 보관) — 번들에 안 들어감
scripts/case_sprites.py   원본 → 합성·정렬·축소·시트 합치기 → frontend/console/src/assets/case/*.webp + manifest.json (V0 원형: design/v2/tools/process_master.py)
```

- 생성: `codex exec -m gpt-5.6-sol -i design/v2/source/master.png -- "<prompts.md의 해당 항목>"` (`-i`는 파일을 여러 개 받으므로 `--` 필수). 저장 위치·투명 배경·편집 동작은 V0에서 확인 — [`design/v2/README.md`](./design/v2/README.md).
- 후처리 스크립트: 편집 프레임을 안 바뀐 쪽 기준으로 정렬하고, 차이 마스크를 열림 연산(얇은 윤곽 잔차 제거)한 뒤 변경 영역만 마스터에 합성 → 알파 bbox로 기준선 정렬 → 화면 사각형 검출(어두운 유리 영역)해 manifest에 `screen` 기록 → 128px 축소 → 시트 합치기 → `manifest.json`. 이미지 처리는 Python + Pillow를 `uv run --with pillow`로 일회성 실행한다(V0에서 확인, 프론트엔드 의존성 없음). 그래서 스크립트 위치도 `frontend/console/scripts/case-sprites.mjs`가 아니라 `scripts/case_sprites.py`. 시트는 커밋된 결과물이고 CI는 스크립트를 돌리지 않는다.
- QA: 마스터와 각 프레임의 어니언 스킨 PNG를 내보내 사람이 확인.

## 6. 단계

### V0 — 스타일 결정 + 파이프라인 스파이크 (1일, 콘솔 코드 변경 없음)

- [x] codex 스파이크: 생성·저장 위치·투명 배경·`-i` 참조 편집 확인 (2026-10-18, `design/v2/README.md`)
- [x] 봇 스타일 3안 생성 (플랫 / 부드러운 셰이딩 / 픽셀 아트) + 64px 비교 시트
- [x] 사용자 선택: **B 부드러운 셰이딩 + CSS 화면 얼굴** (2026-10-18)
- [x] 마스터 확정 → 빈 화면 base + 손 흔들기 포즈 1장으로 64px 검수 **통과** — 결과와 V1에서 고칠 4가지는 [`design/v2/README.md`](./design/v2/README.md) "마스터와 64px 검수"

### V1 — 스프라이트 이식 (1~2일)

- [ ] 나머지 프레임 생성(§4), `scripts/case_sprites.py`, `src/assets/case/` 시트 + manifest
- [ ] `src/components/CaseFigure.svelte` 신설: 몸만 옮긴다. props `expression`, `action`, `idlePlay`, `gaze`, `talking`. 말풍선·질문·cue·타이머는 `CompanionPet.svelte`에 남는다
- [ ] 레이어: 그림자 → 몸체 스프라이트(포즈 전환 시 2장 교차) → 화면 얼굴(CSS, manifest의 `screen`에 맞춤) → CSS LED → z → 배지. 표시 64×64, 버튼 74px·배치 무변경
- [ ] 클래스 계약 유지: `.companion-body.expr-<name>`, `act-<action>`, `play-<kind>`, `.companion-gem`(이제 안테나 LED), `.companion-zzz`, 화면 얼굴의 `companion-eye-*`·`companion-brow-*`. 미어캣 파츠(귀·패치·코·앞발·노트북)는 사라진다
- [ ] 테스트 교체: `<img|<svg` 금지 → "이미지는 `src/assets/case/`에서만, 외부 URL·SVG 몸체 금지", 에셋 총량 상한, manifest 프레임 이름 일치, `--case-gem-*`은 LED에만, 눈 색은 표정마다 같음
- [ ] F6 스크린리더 상태 문장
- [ ] `DESIGN.md` "CASE's expressions" 다시 쓰기(봇 부위·§3 표), `design/v2/README.md`

### V2 — 모션 (1~2일)

| 모션 | 언제 | 방식 | reduced-motion |
|---|---|---|---|
| 깜빡임 | neutral·working·wary | 화면 눈 `scale-y` 120ms | 정지, 뜬 눈 |
| 대기 숨결 | 항상 | 몸 1px 위아래 + 화면 발광 미세 맥동 | 정지 |
| **표정 전환** | 표정이 바뀔 때 | 눈·눈썹 모양 180ms 트랜지션, 포즈는 스프라이트 교차 | 즉시 전환 |
| bounce·shake·tilt | alert·upset·wary 진입 (기존) | 스프라이트 전체 `transform` | 정지 |
| wave | greeting 진입 | `wave` 프레임 시퀀스 | 정지 프레임만 |
| nod | happy 진입 (기존) | 몸 전체 짧은 숙임 `transform` | 정지 |
| 시선 | 포인터 200px 안 (기존) | 화면 눈 연속 이동 | 꺼짐 |
| 하품·윙크·**스캔** | idle play (스캔 신규: 눈이 좌우로 훑고 LED 한 번 깜빡, 백그라운드 턴·목표 모드가 돌 때 확률 상향) | 화면 얼굴 CSS | 꺼짐 |
| **부팅 등장 / 절전 퇴장** | `justArrived`의 greeting 앞 / 숨기기(F2) | 등장: `off` → `base`, 화면에 스캔라인이 한 번 지나가고 눈이 켜짐. 퇴장: 화면이 가운데 점으로 꺼지고(CRT off) `off` 자세 | 즉시 나타남·사라짐 |
| 말하기 | CASE 답 스트리밍 중(F1) | 화면 하단 파형 막대 3개 | 정지, 막대 숨김 |

- 모든 모션은 `.companion-figure` 안에서만, 버튼(히트 영역)은 고정 — #1191 규칙 그대로.
- easing 토큰 하나로 통일(`--case-ease-spring`), DESIGN.md Motion 절에 기록.
- 새 animation/transition 셀렉터는 reduced-motion 블록에 하나씩 나열.

## 7. 기능 추가

P2(Bridge)·P3(돌아왔을 때 요약)는 여전히 가장 큰 기능 공백이라 그대로 간다. 검토하면서 나온 추가 기능은 아래이고, 모두 **읽기 전용·침묵 조건 유지** 불변 조건 안에 있다.

| # | 기능 | 내용 | 붙는 단계 | 크기 |
|---|---|---|---|---|
| F1 | 말하는 CASE | 답 스트리밍 중 화면 파형, 첫 토큰 전에는 스캔(생각 중). **9번째 표정이 아니라 오버레이** — `companionExpression` 표와 `companionAskExpressionOverride` 우선순위 그대로 | V2 | S |
| F2 | 숨기기 (절전) | 말풍선 ⋯ 메뉴에 "1시간 절전 / 다음 알림까지". 화면이 꺼지고 `off` 자세 + 배지만 남는다. 승인 대기·실패가 새로 생기면 부팅해서 나온다. 뷰어별 편의라 `localStorage`(try/catch) | V2 | S |
| F3 | 머지 축하 | 포커스 파이프라인 merge 완료 → happy + 두 판이 벌어졌다 돌아오는 `split`. 기존 SSE에 merge 사실이 실리는지 먼저 확인하고, 없으면 `justFinished` 계열 cue 하나로 추가 | V2 이후 | S–M |
| F4 | 데스크톱 트레이 아이콘 = CASE | `desktop/internal/tray`의 `Idle/Running/NeedsInput/Offline`을 CASE 얼굴로(Offline은 꺼진 화면). 같은 스크립트가 트레이 크기(22·44px) PNG를 따로 내보낸다 | P2 이후 | M |
| F5 | 크기·동작 설정 | 작게/보통, idle play 끄기, 좌/우 위치. 뷰어별 `localStorage`, 말풍선 ⋯ 메뉴 | V2 | S |
| F6 | 스크린리더 상태 문장 | 표정이 바뀌면 시각적으로 숨긴 한 줄을 `aria-live=polite`로 — 말풍선 문장 재사용 | V1 | S |

넣지 않는 것: 음성, 드래그로 아무 데나 옮기기(#1194류 가림 사고 — F5 좌/우 고정 위치로 충분), 랜덤 대사, CASE가 직접 무언가를 실행하는 동작.

## 8. 순서

```
V0 (스타일·얼굴 방식 결정, 검수 통과) → V1 (스프라이트 이식 + F6) → V2 (모션 + F1·F2·F5) → P2 Bridge → F4 → P3 arrival brief → F3
```

- V1에서 몸을 `CaseFigure.svelte`로 분리해 두면 P2는 말풍선 쪽만 고치므로 같은 줄을 다투지 않는다.
- P3의 greet 말풍선은 V2의 "부팅 등장"과 자연스럽게 묶인다.
- V0 검수에서 64px 구분이 안 되면 프레임을 늘리지 말고 스타일을 단순하게 다시 고른다.

검증 범위: `CompanionPet.svelte`는 모든 화면에 마운트된다 → 매 PR **`make console-e2e` 전체** (CLAUDE.md, #1194).

---

### ✅ Checkpoint: Phase 1.5

**자동 확인**
- [ ] `cd frontend/console && npm run check && npm run test:ci`
- [ ] `node --experimental-strip-types --test tests/companionPet.test.ts`
- [ ] `make console-e2e` 전체 통과

**수동 확인** (`make dev-console`)
- [ ] 8표정이 64px에서 LED를 가리고 봐도 구분됨, 포즈 프레임 사이 몸체가 달라 보이지 않음
- [ ] 표정 전환이 튀지 않음, 승인 대기 → alert + 안테나 + bounce
- [ ] 탭을 10분 넘게 숨겼다 돌아오면 부팅하며 등장해 손 흔듦
- [ ] CASE에 질문 → 첫 토큰 전 스캔, 스트리밍 중 화면 파형
- [ ] "동작 줄이기" 켬 → 모든 반복 모션 정지, 표정은 그대로
- [ ] Retina·일반 해상도 모두 선명, 첫 동작 재생 때 깜빡임 없음, Chat 레일·Focus 거터를 가리지 않음

**통과하면 사용자 확인 후 P2로.**
