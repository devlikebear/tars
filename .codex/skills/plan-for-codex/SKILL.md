---
name: plan-for-codex
description: "요구사항을 Codex가 직접 실행할 작업지시서(Work Order)로 변환하는 설계/계획 수립 스킬. 다음 상황에서 트리거: (1) 사용자가 새 기능 개발을 요청하고 구현 전 계획이 필요할 때, (2) '/plan'을 호출할 때, (3) 요구사항을 작업 단위로 분해해야 할 때."
---

# Plan

요구사항을 Codex가 바로 구현할 수 있는 Work Order 세트로 변환한다.

## Input

- 사용자의 요구(자연어)
- 제약(호환성, 일정, 변경 금지 영역 등)

## Output

작업지시서 세트(최대 3개). 각 작업은 [Work Order Template](../implement/references/work-order.md) 형식으로 작성한다.

## Rules

1. 한 작업은 30~90분 규모로 제한
2. Touch points는 5개 이하
3. Acceptance criteria는 "검증 가능한 문장"으로 작성
4. Verification commands를 반드시 포함
5. API 변경이 필요하면 명시적 "허용"이 없으면 금지로 간주
6. 계획 완료 후 `/implement`로 같은 Work Order를 기준으로 직접 구현 진행
