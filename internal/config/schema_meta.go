package config

// configLiveApplyFields is intentionally narrow. Today /v1/admin/config/values
// patches YAML only, so runtime behavior changes require a process restart.
// Fields can opt into live apply here when a future handler updates the
// running subsystem immediately after saving.
var configLiveApplyFields = map[string]bool{}

func withConfigFieldMeta(fields []FieldMeta) []FieldMeta {
	defaults := schemaDefaultValues()
	for i := range fields {
		fields[i].DefaultValue = defaults[fields[i].Key]
		fields[i].RequiresRestart = !configLiveApplyFields[fields[i].Key]
	}
	return fields
}

func schemaDefaultValues() map[string]any {
	cfg := Default()
	applyDefaults(&cfg)
	return ConfigToMap(cfg)
}
