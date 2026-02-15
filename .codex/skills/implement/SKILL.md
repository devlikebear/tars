---
name: implement
description: "계획된 작업을 Codex가 직접 구현하는 스킬. 다음 상황에서 트리거: (1) '/implement'를 호출할 때, (2) '/plan' 완료 후 구현 단계에 진입할 때, (3) 코드 생성/수정 작업을 즉시 실행해야 할 때."
---

# Implement

Codex가 계획 수립부터 코드 변경, 검증까지 직접 수행한다.

원칙: "작게, 안전하게, 테스트로 증명"

## Workflow

### A. 사전 정리

1. 요구를 1~3개의 작업 단위로 분해 (각 30~90분 규모)
2. 범위 제한(Non-goals) 명시
3. 수정 예상 파일(최대 5개) 제시
4. 검증 커맨드 명시 — Go: `go test ./...` 또는 `make test`

### B. 직접 구현

구현 전 [Work Order Template](references/work-order.md) 형식으로 작업 내용을 확정한다.

```
아래 Work Order를 기준으로 직접 구현한다.

## Goal
<한 문단>

## Non-goals
<하지 말 것>

## Touch points (<=5)
<file1>, <file2>

## Steps
- [ ] Step 1
- [ ] Step 2

## Acceptance criteria
- [ ] AC1
- [ ] AC2

## Verification commands
<go build ./... && go vet ./...>
```

규칙:
- 1회 실행에 1 작업 단위만 수행
- 큰 리팩터링이 필요해지면 중단하고 최소 변경 단위로 재계획
- 실패 시 최소 수정으로 재시도(최대 2회), 그래도 실패하면 중단하고 원인 보고

### C. 결과 검수

1. Acceptance Criteria 충족 여부 체크
2. 변경 범위가 Non-goals를 침범했는지 확인
3. `make test` 실행하여 테스트 통과 확인
4. 보안: 키/토큰 노출, 인젝션 위험 확인
5. 문제 발견 시 `/review`로 진행하여 수정 지시서 생성

## 최종 출력 포맷

- 완료 작업 요약
- 변경 파일 목록
- 테스트 결과
- 리스크/주의점
- 다음 작업 제안(있으면)
