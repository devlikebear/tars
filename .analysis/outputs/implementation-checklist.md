# 구현 체크리스트

## 문서

- [x] README에서 KB Wiki 광고 제거
- [x] GETTING_STARTED Memory 설명 최신화
- [x] CLAUDE.md CLI/Reflection/Memory 설명 최신화
- [x] tutorials 15/19/20/21/22를 현재 활성 surface 기준으로 갱신
- [x] tutorial roadmap에서 project/dashboard/autopilot/TUI를 제거된 레거시로 분리

## 코드 리뷰 후속

- [ ] #406 config metadata/provider cleanup
- [ ] #407 frontend component/API type debt
- [ ] #408 CLI/client URL resolution
- [ ] #409 service/LaunchAgent plumbing
- [ ] #410 KB Wiki removal residue
- [ ] #411 init/assistant/hub polish
- [ ] #412 skillhub update + assistant popup escaping
- [ ] #413 workspace file preview + reset failures
- [ ] #414 agentruntime route/client/docs canonicalization
- [ ] #415 state persistence/default contracts
- [ ] #416 skill runtime mirror companion-copy failures
- [ ] #417 `--serve-api=false` intent
- [ ] #418 usage-signaled feature decisions

## 검증

- [x] `git diff --check`
- [x] stale token scan: `MYCLAW`, `localhost:8080`, `knowledge base`, `internal/server`
- [ ] source-analyzer summary/search-index generation
- [x] wiki dry-run
- [x] wiki publish
