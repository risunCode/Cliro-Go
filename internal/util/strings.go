package util

// FirstNonEmpty returns the first non-empty string from the provided values.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
