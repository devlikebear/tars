# 클론 코딩 가이드

## 목표

TARS 전체를 한 번에 복제하지 말고, 작은 runtime을 순서대로 확장한다.

## 구현 순서

1. Cobra root와 `serve` 명령을 만든다.
2. session index와 transcript JSONL을 만든다.
3. prompt builder와 tool registry를 만든다.
4. `/v1/chat` SSE endpoint와 agent loop를 만든다.
5. OpenAI-compatible provider adapter를 붙인다.
6. YAML/env config loader를 붙인다.
7. file/exec tool을 추가한다.
8. transcript compaction과 memory search를 붙인다.
9. skill loader와 MCP client를 optional extension으로 둔다.
10. one-shot CLI와 웹 콘솔 중 하나를 사용자-facing UI로 선택한다.
11. cleanup 같은 위험 작업에는 approval pattern을 둔다.
12. 긴 작업은 synchronous chat이 아니라 agent runtime run으로 분리한다.

## 주의할 경계

- domain-specific 기능을 builtin tool로 늘리지 않는다.
- user-facing registry와 system surface registry를 섞지 않는다.
- background job은 직접 Go interface를 통해 좁게 의존하게 한다.
- cleanup/delete/write 같은 작업은 plan/approval/apply 단계로 나눈다.
- legacy route를 추가할 때는 canonical route와 제거 계획을 같이 문서화한다.
