# 모듈: Frontend Console

## 역할

`frontend/console`은 TARS의 주 UI다. Chat, Memory, System Prompt, Ops, Pulse, Reflection, Extensions, Config 화면을 제공한다.

## 주요 파일

- `frontend/console/src/App.svelte`
- `frontend/console/src/lib/router.ts`
- `frontend/console/src/lib/api.ts`
- `frontend/console/src/lib/types.ts`
- `frontend/console/DESIGN.md`

## 관찰

- Svelte 5 runes를 사용한다.
- DESIGN.md가 색상/토큰/컴포넌트 규칙의 source of truth다.
- large component와 backend mirror type debt는 #407에서 추적한다.
