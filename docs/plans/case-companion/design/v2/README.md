# CASE v2 — 봇 래스터 스프라이트 V0 결과

Phase 1.5([`../../phase-1.5-visual-motion.md`](../../phase-1.5-visual-motion.md)) V0의 산출물. 여기 있는 PNG는 **레퍼런스**이고 콘솔 번들에 들어가지 않는다. 모든 이미지는 codex CLI의 이미지 생성(AI)으로 만들었다 — 프롬프트는 [`prompts.md`](./prompts.md). 저장소 용량 때문에 원본(1024px)을 512px로 줄여 두었다.

캐릭터는 **동반 로봇**이다. 같은 날 먼저 뽑았던 미어캣 3안은 캐릭터 변경(phase 문서 §0)으로 지웠다.

## 스파이크 결과 (2026-10-18)

| 확인 항목 | 결과 |
|---|---|
| 비대화형 `codex exec`에서 이미지 생성 | **된다.** 한 장에 약 2분, 3~4장 병렬 실행도 문제없음 |
| 모델 | `~/.codex/config.toml`의 `gpt-6.1-sol`은 ChatGPT 계정에서 400으로 거절된다. `-m gpt-5.6-sol`로 실행 |
| 저장 위치 | `~/.codex/generated_images/<thread-id>/exec-<uuid>.png`. 프롬프트로 시키면 codex가 작업 폴더로 복사해 준다 |
| 크기 | 정사각형 요청 시 1024×1024(가끔 1254×1254), 미지정 시 1024×1536 |
| 투명 배경 | **처음부터 RGBA로 나온다** (모서리 알파 0 확인) |
| `-i` 참조 편집 | **된다.** 눈만 감긴 프레임이 나오고 자세·색·구도는 유지 (`compare/edit-blink.png`). 단, 픽셀 단위로 보존하지는 않는다 — 바꾸라고 한 곳 밖에도 잔차가 흩어져 있다 (`compare/edit-blink-diff.png`, 차이 40 초과 픽셀 마스크) |
| `-i` 사용법 | 파일을 여러 개 받는 옵션이라 프롬프트 앞에 `--`가 없으면 프롬프트를 파일명으로 읽고 "No prompt provided"로 끝난다 |

계획에 반영한 것:
1. **패치 합성**: 편집 결과를 통째로 쓰지 않고, 바뀌어야 할 영역(팔·안테나)만 마스크로 잘라 마스터 위에 합성한다.
2. **전신 구도로 충분**: 봇은 얼굴 화면이 몸의 1/3이라 64px 전신에서도 눈이 또렷하다 (`compare/bots-64px-full-vs-bust-4x.png`, 각 안 왼쪽 전신·오른쪽 상반신, 4배 확대). 미어캣 때 필요했던 상반신 크롭은 쓰지 않는다 — 팔·발 포즈가 보여야 한다.

## 봇 스타일 3안

`compare/bots-256-64-32.png` — 왼쪽부터 A·B·C, 각각 256/64/32px, 콘솔 배경 `#141414` 위.

| 안 | 64px에서 | 장점 | 단점 |
|---|---|---|---|
| A 플랫 벡터 | 눈은 또렷, **몸체가 배경에 묻힘** | 콘솔의 플랫 UI와 맞음, 패치 합성이 가장 쉬움, 용량 최소 | 짙은 회색 몸이 `#141414`와 대비가 낮아 실루엣이 약함 — 쓰려면 몸 톤을 한 단계 밝혀 다시 뽑아야 함 |
| **B 부드러운 셰이딩** | 셋 중 가장 잘 읽힘. 금속 하이라이트가 윤곽을 잡아 줌 | 가장 고급스럽고 "예쁨", 어두운 배경에서 실루엣이 산다 | 반사광 때문에 패치 합성 경계를 조심해야 함, 용량이 조금 큼 |
| C 픽셀 아트 | 축소하면 픽셀이 64px 격자와 안 맞아 **뭉개짐** | 레트로 터미널 느낌이 개발 도구와 어울림 | 생성된 "픽셀"이 실제 격자에 정렬돼 있지 않아 64px에서 장점이 사라진다. 쓰려면 정확히 64×64로 다시 찍는 후처리(격자 추정)가 필요 |

권장: **B**. 얼굴을 CSS 화면으로 그리는 권장안(phase 문서 §2.2)이면 마스터에서 눈을 지운 "빈 화면" 판을 `-i` 편집으로 한 장 더 만든다.

## 마스터와 64px 검수 (2026-10-18)

결정: **B 부드러운 셰이딩**, 얼굴은 **CSS 화면** 방식.

| 파일 | 내용 |
|---|---|
| `source/master.png` | 마스터 = `bot-b` |
| `source/master-blank.png` | 마스터에서 `-i` 편집으로 눈을 지운 빈 화면 판 (V1의 `base` 프레임 원본) |
| `source/pose-wave-raw.png` | 마스터에서 `-i` 편집으로 오른팔(화면 왼쪽)을 든 포즈, 합성 전 원본 |
| `review/frames/base.png`·`wave.png` | 128px 셀. `wave`는 팔 영역만 `master-blank` 위에 합성한 것 |
| `review/frames/manifest.json` | 128px 기준 화면 사각형 `screen: [x0, y0, x1, y1]`, 안테나 LED `led: [cx, cy, r]` (자동 검출) |
| `review/review.html` | 8표정을 CSS 화면 얼굴 + CSS LED로 얹은 검수 페이지 (정적 서버로 연다 — 아래) |
| `compare/v0-review-64-128.png` | 검수 페이지 1배율 스크린샷 |
| `compare/v0-review-64px-2x.png` | 64px 두 줄(LED 있음/가림)을 2배 최근접 확대 |
| `compare/wave-onion-mask.png` | 왼쪽: 어니언 스킨(흰색=같음, 시안=새 팔, 빨강=지운 팔), 오른쪽: 합성 마스크 |
| `tools/process_master.py` | 화면·LED 검출, 정렬, 팔 영역 합성, 128px 축소, manifest (Pillow·numpy) |
| `tools/run-bot.sh`·`run-master.sh` | V0에서 돌린 codex 생성 스크립트 (작업 폴더 구조는 당시 scratch 기준) |

검수 결과 — **통과**:
- 8표정이 64px에서 **LED를 가려도 모양으로 구분**된다 (`compare/v0-review-64px-2x.png` 아랫줄).
- 화면 얼굴이 CSS라 64px에서도 발광 눈이 번지지 않고 선명하다. 몸체 금속 하이라이트가 `#141414` 위 실루엣을 잡아 준다.
- 포즈 합성: 편집이 몸 전체를 1~2px 어긋나게 다시 그리므로(정렬 오프셋 `[-2, -1]`), 그대로 쓰면 윤곽선이 떨린다. 차이 마스크를 **열림 연산(MinFilter 9 → MaxFilter 25)** 한 뒤 합성하니 팔만 남고 몸체는 두 프레임에서 픽셀 단위로 같아졌다 (`compare/wave-onion-mask.png`).
- 화면 사각형·LED 위치 자동 검출이 맞았다 — CSS 얼굴과 LED가 제자리에 얹힌다.

V1에서 고칠 것:
1. 64px에서 **눈썹이 1px 남짓**이라 alert↔neutral 차이가 눈 크기에 기대고 있다. 눈썹을 64px 기준 최소 2px로 두껍게 하고, alert에는 계획대로 포즈 프레임(안테나 뻗음·팔 벌림)을 더해 세 군데가 다르게 한다.
2. working의 진행 점 3개가 64px에서 거의 안 보인다 → 점 대신 짧은 진행 막대(좌우로 흐르는 1줄)로.
3. 로봇이 64px 셀 높이의 약 75%만 쓴다. 셀을 꽉 채우도록 크롭 여백을 줄이면 화면이 조금 더 커진다.
4. 후처리는 `sharp` 대신 **Python + Pillow를 `uv run --with pillow`로 일회성 실행**하는 쪽이 낫다 — 프론트엔드 의존성을 늘리지 않고, 이번 V0에서 그대로 동작했다.

## 다시 만드는 법

```bash
# 비교 시트 (ffmpeg 필요)
zsh compare/compare.sh out.png source/bot-a.png source/bot-b.png source/bot-c.png
zsh compare/pixels.sh  out.png source/bot-a.png source/bot-b.png source/bot-c.png

# 프레임·manifest 다시 만들기 (1024px 원본 필요 — 저장소에는 512px만 있다)
uv run --no-project --with pillow --with numpy python -I tools/process_master.py <blank-1024.png> <pose-1024.png> review/frames

# 검수 페이지 (file:// 에서는 이미지가 안 뜨는 브라우저가 있어 정적 서버로)
python3 -m http.server 8765 --bind 127.0.0.1 -d review   # → http://127.0.0.1:8765/review.html
```
