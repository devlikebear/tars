# Migration Notes

## 2026-02-16

### tarsd config: tool selector/policy keys removed

Runtime now always injects all registered tools per turn (OpenClaw parity).  
The following keys were removed from config parsing and are ignored if present:

- `tools_profile`
- `tools_allow`
- `tools_deny`
- `tools_by_provider_json`
- `tool_selector_mode`
- `tool_selector_max_tools`
- `tool_selector_auto_expand`

### What to do

1. Remove the keys above from `config/standalone.yaml` (or your custom tarsd config).
2. Keep using optional tool flags only:
   - `tools_apply_patch_enabled`
   - `tools_web_fetch_enabled`
   - `tools_web_search_enabled`
   - `tools_web_search_api_key`
3. Restart `tarsd`.

### Why this changed

- Selector/policy config no longer matched runtime behavior.
- Removing dead config paths reduces confusion and maintenance cost.
