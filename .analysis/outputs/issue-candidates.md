# 이슈 후보

이 목록은 `docs/code-review/status.md`와 이번 문서 최신화 중 확인한 잔여 모호성을 합친 것이다.

## High

### #410 KB Wiki removal residue

KB Wiki user-facing surface는 제거됐지만 bootstrap template, workspace initializer, `/v1/memory/kb/*` route 잔여물이 남아 있다. README/tutorials에서는 더 이상 기능으로 광고하지 않도록 정리했다.

### #414 Agent Runtime route/client/docs canonicalization

Run API는 `/v1/agentruntime/runs`가 canonical이어야 하지만 `/v1/agent/runs` alias와 `/v1/agent/agents` list endpoint가 남아 있다. 외부 client와 docs를 한 방향으로 정리해야 한다.

## Medium

### #408 CLI/client URL resolution

root console open, internal client, public `pkg/tarsclient`가 URL 조립/default를 각자 처리한다. Default URL과 `/console` suffix 정책을 한 곳으로 모을 수 있다.

### #409 macOS service + LaunchAgent plumbing

service stop/status와 config health, custom launchd label/domain, server restart detection이 서로 다른 helper에서 움직인다.

### #415 State persistence/default contracts

extensions disabled-state, ops workspace default, ops/usage atomic writes는 운영 안정성에 직접 영향을 준다.

## Product/Intent Questions

### #417 `--serve-api=false`

서버 API를 끄는 모드가 실제 사용자 시나리오인지, 레거시 플래그인지 결정이 필요하다.

### #418 usage-signaled feature decisions

Q-011/Q-012/Q-014/Q-015/Q-017/Q-018은 사용량 신호가 쌓인 뒤 유지/제거/간소화 결정을 내려야 한다.
