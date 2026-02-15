---
name: refactor
description: "코드베이스 리팩토링을 안전하게 수행하는 스킬. 다음 상황에서 트리거: (1) '/refactor'를 호출할 때, (2) 코드 품질 개선, 구조 정리, 중복 제거가 필요할 때, (3) 기술 부채 해소가 필요할 때. 기능 변경 없이 내부 구조만 개선하며, 각 단계마다 테스트로 안전성을 검증한다."
---

# Refactor

기능 변경 없이 코드 내부 구조를 개선한다. 안전성을 최우선으로 하며, 작은 단위로 진행하고 각 단계마다 테스트로 검증한다.

원칙: "작게, 안전하게, 테스트로 증명"

## Input

- 리팩토링 대상 (파일/패키지/함수 등)
- 리팩토링 목적 (가독성, 성능, 유지보수성 등)
- 제약사항 (API 변경 금지, 특정 파일 수정 금지 등)

## Workflow

### 1. 사전 분석 (Claude Code가 수행)

1. 현재 코드 상태 파악
   - 파일 구조, 의존성, 테스트 커버리지 확인
   - `go test ./...` 실행하여 현재 테스트가 통과하는지 확인
2. 리팩토링 범위 결정
   - 영향 범위 분석 (최대 5개 파일)
   - 단계별 작업 분해 (각 30분 이하)
3. 리팩토링 패턴 선택
   - [Refactoring Patterns](references/refactoring-patterns.md) 참고
   - 적용할 기법 명시 (Extract Function, Rename Variable 등)

### 2. 리팩토링 실행 (codex-implementer에 위임)

Task 도구로 `codex-implementer` 서브에이전트를 호출한다. 프롬프트는 [Work Order Template](../implement/references/work-order.md) 형식을 사용:

```
Task(subagent_type="codex-implementer", prompt="""
아래 리팩토링 Work Order를 구현하라.

## Goal
<한 문단: 무엇을 어떻게 개선할 것인가>

## Non-goals
- 기능 변경 금지
- API 시그니처 변경 금지 (명시적 허용이 없는 경우)
- 새 기능 추가 금지
- 대량 포맷팅 변경 금지

## Touch points (<=5)
<file1>, <file2>

## Refactoring Pattern
<Extract Function | Inline Function | Rename | Move | etc.>

## Steps
- [ ] 테스트가 통과하는지 확인
- [ ] 리팩토링 적용
- [ ] 테스트 재실행하여 동작 동일 확인

## Acceptance criteria
- [ ] 모든 기존 테스트가 통과한다
- [ ] 기능 동작이 변경되지 않았다
- [ ] 코드 가독성/구조가 개선되었다

## Verification commands
make test
""")
```

규칙:
- 1회 호출에 1개 리팩토링만 수행
- 테스트가 실패하면 즉시 롤백
- 기능 변경이 감지되면 중단하고 보고

### 3. 검증 (Claude Code가 수행)

1. [Refactoring Checklist](references/refactoring-checklist.md) 기준으로 점검
2. 테스트 실행 (`make test`)
3. diff 확인 - 의도하지 않은 변경 없는지 확인
4. 문제 발견 시 롤백 또는 수정

### 4. 다음 단계

- 단계적 리팩토링이면 다음 작업 진행
- 완료되면 변경사항 커밋

## Output

- 개선 사항 요약
- 변경 파일 목록
- 테스트 결과
- 추가 리팩토링 제안 (있으면)

## Safety Rules

1. **기능 변경 절대 금지**: 리팩토링은 내부 구조만 개선
2. **테스트 필수**: 각 단계마다 `make test` 실행
3. **작은 단위**: 한 번에 하나의 리팩토링만 수행
4. **API 보존**: 명시적 허용 없이 public API 변경 금지
5. **롤백 준비**: 문제 발생 시 즉시 원복
6. **의존성 주의**: 다른 모듈에 영향 최소화
