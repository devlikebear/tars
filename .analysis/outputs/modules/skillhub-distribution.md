# 모듈: Skill Hub 배포

## 핵심 파일

- `cmd/tars/skill_main.go`
- `cmd/tars/plugin_main.go`
- `internal/skillhub/registry.go`
- `internal/skillhub/install.go`
- `internal/skillhub/types.go`
- `internal/skill/mirror.go`

## 역할

이 모듈은 원격 registry에서 skill과 plugin을 검색하고, workspace에 설치하고, 설치 상태를 추적하는 배포 계층이다. runtime loader와는 별개로 "무엇을 내려받을지"를 결정한다.

## 기본 흐름

1. `skill_main.go` 또는 `plugin_main.go`가 CLI 입력을 받는다.
2. `registry.go`가 원격 registry JSON을 내려받아 search/info 결과를 만든다.
3. `install.go`가 `skills/` 또는 `plugins/` 아래에 파일을 기록한다.
4. 설치 결과는 workspace `skillhub.json`에 기록된다.
5. 서버 runtime에서는 `extensions.Manager`와 `skill.MirrorToWorkspace`가 실제 로딩 경로를 다시 만든다.

## 중요한 관찰

- registry와 파일 전송은 `raw.githubusercontent.com`을 직접 사용한다.
- skill은 `RequiresPlugin` 메타데이터로 companion plugin 의존성을 알릴 수 있다.
- companion file 다운로드는 현재 best-effort 라서 설치 completeness 보장은 약하다.
- runtime path는 Hub 설치 경로와 다를 수 있다. agent는 보통 mirror된 runtime path를 읽는다.
