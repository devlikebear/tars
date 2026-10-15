# Design system — Focus × Crew · CASE

`focus-crew/design/`와 `case-companion/design/`의 HTML 시안이 함께 쓰는 폰트와 토큰.

| 파일 | 내용 |
|---|---|
| `tokens.css` | Monolith 표면·텍스트·신호색(`--fm-*`), 크루 봇 색(`--crew-*`), CASE 미어캣 색(`--case-*`), 폰트 스택 |
| `fonts.css` | 로컬 `@font-face` (Geist 400/500/600, Geist Mono 400/500, Martian Mono 400/500, latin) |
| `fonts/` | woff2 7개 + OFL 라이선스 3개. 출처: `@fontsource/geist`, `@fontsource/geist-mono`, `@fontsource/martian-mono` 5.3.0 |

## 콘솔에 옮길 때

- **폰트**: 두 가지 중 하나를 고른다.
  1. `cd frontend/console && npm i @fontsource/geist @fontsource/geist-mono @fontsource/martian-mono` 후 필요한 굵기만 import (권장 — 오프라인·데스크톱 앱에서도 동작)
  2. 현재 `app.css`처럼 Google Fonts `@import`
  여기 `fonts/` 폴더는 시안을 오프라인으로 열기 위한 사본이다. 레포 소스로 옮기지 말고 1번으로 설치한다.
- **토큰 스코프**:
  - `--fm-*`, `--crew-*`는 focus 라우트 루트(`.focus-monolith`) 아래에만 선언한다 (focus-crew phase 4). 전역 `:root`에 두면 다른 화면이 바뀐다.
  - `--case-*`는 `CompanionPet.svelte`의 `<style>` 안에만 선언한다 (case-companion phase 1).
- 한글 글리프는 세 폰트에 없다. 스택 끝의 시스템 폰트로 대체되며, 현재 콘솔(IBM Plex)과 같은 동작이다.
- 대비: `--fm-text-3`(#8a877d)이 `--fm-surface`(#121211) 위 작은 글자의 최저선이다(4.5:1 이상). `--fm-text-ghost`는 장식 전용이다.
