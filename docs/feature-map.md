# Feature Map

Last reviewed: 2026-10-06. Start an isolated instance with `make build` then `bin/tars serve --config <sandbox-config>`; use a throwaway workspace for verification.

## Computer use

### Desktop action tool
- Kind: chat tool
- Reach: console chat or `POST /v1/chat`; ask to call `computer_use(goal, app?, inputs?, max_steps?)`.
- Drive: with cua-driver running and permitted, ask Calculator to clear and calculate 2 + 3. The default backend resolves the `computer_use` role to light; no Jev configuration is needed.
- Healthy result: `status: done`, visible result 5, action trace followed by a final observation, and LLM input/output usage. Risky controls/secure typing return `needs_confirmation`; answer via `resume` and `confirm`.
- Evidence: capture tool JSON and the final Calculator observation. Automated driver/provider boundary coverage: `go test ./internal/tarsserver -run 'TestComputerUse.*EndToEnd' -v`; live tool coverage: `TARS_COMPUTER_USE_LIVE_CONFIG=<config> CUA_DRIVER_PATH=<driver> go test -tags integration ./internal/tarsserver -run TestLiveComputerUseLight -v`.

### Backend configuration and readiness
- Kind: configuration / command
- Reach: `tools.computer_use` in YAML and `tars doctor --config <config>`.
- Drive: check default `enabled: true`, `backend: llm`; set `enabled: false` to disable or `backend: jev` with `jev.base_url` to switch. Set `expose_values: false` to withhold field contents.
- Healthy result: doctor reports the selected LLM tier/model or Jev location and missing driver guidance. Jev keeps confidence gates. Missing setup returns `unavailable`; no computer action runs.
- Evidence: doctor output and `go test ./internal/config ./cmd/tars -run 'TestComputerUse|TestCheckDoctorComputerUse' -v`.
