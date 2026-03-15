package config

func merge(dst *Config, src Config) {
	mergeConfigInputFields(dst, src, configInputFields)
}
