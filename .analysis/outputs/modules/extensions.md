# 모듈: Extensions

## 역할

Skill, Plugin, MCP, Skill Hub는 core prompt/tool surface를 키우지 않고 기능을 on-demand로 불러오기 위한 확장 시스템이다.

## 주요 파일

- `internal/skill/*`
- `internal/plugin/*`
- `internal/extensions/*`
- `internal/mcp/*`
- `internal/skillhub/*`
- `cmd/tars/skill_main.go`
- `cmd/tars/plugin_main.go`
- `cmd/tars/mcp_main.go`

## 관찰

- domain-specific 기능은 core Go tool이 아니라 skill + companion CLI로 보내는 것이 기본 정책이다.
- builtin Go plugin과 plugin HTTP route surface는 제거됐다.
- Skill runtime mirror companion-copy failure는 #416에서 추적한다.
