package tool

func resolvePositiveBoundedInt(defaultValue, maxValue int, override *int) int {
	value := defaultValue
	if override != nil {
		value = *override
	}
	if value <= 0 {
		value = defaultValue
	}
	if maxValue > 0 && value > maxValue {
		value = maxValue
	}
	return value
}
