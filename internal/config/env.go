package config

func applyEnv(cfg *Config) {
	applyConfigInputFieldsFromEnv(cfg, configInputFields)
}
