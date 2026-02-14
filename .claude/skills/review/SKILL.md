---
name: review
description: "코드 변경사항을 리뷰하고 품질/보안/회귀 위험을 점검하는 스킬. 다음 상황에서 트리거: (1) '/review'를 호출할 때, (2) '/implement' 완료 후 검수 단계에 진입할 때, (3) diff 기반 코드 리뷰가 필요할 때. 리뷰 결과 수정이 필요하면 수정 Work Order를 생성해 '/implement'로 재위임한다."
---

# Review

변경사항(diff)을 기준으로 기능/품질/보안/회귀 위험을 점검하고, 수정 지시를 구체적으로 만든다.

## Input

- 변경된 파일 목록 또는 `git diff`
- 요구사항(있으면)

## Checklist

[Review Checklist](references/review-checklist.md)를 기준으로 점검한다.

## Output

- **판정**: OK / 요청 수정 / 블로킹
- **블로킹 이슈**: 정확한 파일/라인 위치
- **개선 권고**: 선택
- **수정 Work Order**: 문제 발견 시 아래 템플릿으로 생성 후 `/implement`로 재위임

수정이 필요하면 [Work Order Template](../implement/references/work-order.md) 형식으로 작성 후 `/implement`로 재위임한다.
