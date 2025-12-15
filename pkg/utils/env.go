package utils

// EnvFallback returns the prefixed value if not empty, otherwise returns the fallback value.
// This is useful for supporting both prefixed env vars (e.g., DB_ONBOARDING_HOST) with
// fallback to non-prefixed (e.g., DB_HOST) for backward compatibility.
func EnvFallback(prefixed, fallback string) string {
	if prefixed != "" {
		return prefixed
	}

	return fallback
}

// EnvFallbackInt returns the prefixed value if not zero, otherwise returns the fallback value.
// This is useful for supporting both prefixed env vars with fallback to non-prefixed
// for backward compatibility.
func EnvFallbackInt(prefixed, fallback int) int {
	if prefixed != 0 {
		return prefixed
	}

	return fallback
}
