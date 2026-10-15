# Phase 5 — 크루 해산 · 라이브러리 · 기억 (D5)

> Roadmap: [roadmap.md](./roadmap.md) · 선행: Phase 1 (Phase 2~4와 독립적으로 진행 가능)
> 목표: 미션이 끝나면 TARS가 봇별 기여를 정리하고 **보관/은퇴**를 제안한다. 일회성 봇은 사라지고, 쓸모 있던 봇은 라이브러리에 남는다. 스크라이브가 남길 기억은 기존 메모리 인박스 후보로 올린다.

## 완료 조건

- G4가 열리면(머지 없는 템플릿은 마지막 단계 진입 시) 해산 패널이 보인다: 봇별 기여 요약, 보관/은퇴 토글(채용 봇 기본 은퇴, 라이브러리 봇 기본 보관).
- 파이프라인이 끝나면(`FinishedAt` 기록 시) 모든 크루 워커 세션 삭제. `Keep == true`인 채용 봇은 라이브러리 파일로 저장.
- 라이브러리 봇은 사용 통계(미션 수, 최근 사용)가 갱신된다.
- 스크라이브 봇이 있으면 기억 후보 최대 3개를 메모리 인박스에 제안하고, 해산 패널에서 뺄 수 있다.
- Focus 홈의 크루 라이브러리 관리: 목록·삭제·이름 변경(admin).

## 작업

### A. 순수 로직
- [ ] `crew.go`: `SetKeep(p, botID, keep bool) (Pipeline, error)` — 끝난 파이프라인은 거부
- [ ] `CrewDebrief(p, log) []DebriefRow` — 봇별: 참여 단계, 노트 수, 기각/채택 주장, finding(합의/이견), 소요 시간 (crew log에서 계산)
- [ ] `crew_stats.go`: `<workspace>/focus-crew/.stats.json` `{bot_id: {missions, last_used}}` 읽기/갱신 (atomicwrite 사용)
- [ ] 테스트: 기본값(채용=은퇴), debrief 집계, stats 갱신 멱등

### B. 서버
- [ ] `POST /v1/focus/pipelines/{id}/crew/{bot}/keep` `{keep}` (admin)
- [ ] 종료 훅: 파이프라인이 `Finished`가 되는 곳(기존 `finished.go`/드라이버의 worktree 정리 지점)에서 `crewTeardown(p)`:
  - 모든 `CrewMember.SessionID` → `sessions.Delete`
  - `Origin == hired && Keep == true` → `SaveBot` (id 충돌 시 `-2` 접미사, 템플릿 규칙)
  - stats 갱신
  - 실패는 로그만 남기고 파이프라인 종료를 막지 않음
- [ ] 중지(stop)된 파이프라인도 같은 정리 (보관 없음)
- [ ] 기억 후보: 스크라이브 봇이 있으면 G4 오픈 시 서브턴 1회 — "이번 미션에서 이 레포에 대해 앞으로도 유효한 사실 ≤3개" → `<crew-note>` claims → `pkg/memory.AppendInboxCandidateIfNew` (provenance: focus pipeline id). 패널의 "빼기" = `ReviewMemoryCandidate`(reject)
- [ ] 라이브러리 관리: `PUT /v1/focus/crew/bots/{id}`, `DELETE /v1/focus/crew/bots/{id}` (admin, 내장 봇은 삭제 불가)
- [ ] 테스트: 종료 시 세션 삭제, 보관 저장, 중지 시 정리, 기억 후보 생성/거부

### C. 콘솔
- [ ] `FocusCrewDebrief.svelte` (D5 오른쪽): 봇 행(아바타·기여·보관/은퇴 토글), 기억 후보 목록(빼기). G4 카드(`FocusMergeSummary`)는 왼쪽에 기존 그대로
- [ ] 은퇴 예정 행은 흐리게(애니메이션은 reduced-motion 존중)
- [ ] `FocusHome` 크루 라이브러리 패널에 관리 메뉴 (admin일 때만)
- [ ] i18n, 단위 테스트(debrief 표시 모델), e2e "G4 → 채용 봇 보관 → 머지 → 라이브러리에 등장"

### D. 문서
- [ ] ADR §11.3 Lifecycle: 보관/은퇴 기본값, 정리 시점, 기억 후보는 사람 승인 전까지 메모리 아님
- [ ] `docs/focus-templates.md`: 라이브러리 파일 관리 설명
- [ ] feature-map "Crew debrief & library"

---

### ✅ Checkpoint: Phase 5

**자동 확인**
- [ ] `make test`, `make test-one TEST_NAME='Debrief|Keep|CrewTeardown|CrewStats' PKG=./internal/...`
- [ ] 콘솔 단위·e2e, `npm run check`
- [ ] `make lint-diff && make test-diff && make arch-check`

**수동 확인**
- [ ] 전체 미션 1회 완주: 편성 → 허들 → G1 → build → 상호검토 → G2 → PR → G4 → 해산(채용 봇 보관) → 머지
- [ ] 끝난 뒤 워커 세션이 세션 목록/디스크에 남지 않음
- [ ] 다음 새 태스크에서 보관한 봇이 소환 후보로 나옴
- [ ] 메모리 인박스에 후보가 있고, 빼기 한 것은 거부 상태

**통과하면 로드맵 완료. 사용자와 회고 후 다음 후보(`@봇` 라우팅, pr_review 교차검토 등) 결정.**
