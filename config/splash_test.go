package config

import "testing"

// TestGetSplashDefaultsToRandom pins the normalization: nil receiver, empty
// field, and unknown (hand-edited) values all resolve to random mode.
func TestGetSplashDefaultsToRandom(t *testing.T) {
	var nilCfg *Config
	for name, got := range map[string]string{
		"nil config": nilCfg.GetSplash(),
		"empty":      (&Config{}).GetSplash(),
		"unknown":    (&Config{Splash: "sparkles"}).GetSplash(),
	} {
		if got != SplashRandom {
			t.Errorf("%s: GetSplash() = %q, want %q", name, got, SplashRandom)
		}
	}
}

// TestGetSplashRoundTripsVariants guards every settings-panel option: a pinned
// pattern name must come back verbatim, never fall through to random.
func TestGetSplashRoundTripsVariants(t *testing.T) {
	for _, v := range SplashVariants() {
		if got := (&Config{Splash: v}).GetSplash(); got != v {
			t.Errorf("GetSplash() = %q, want %q", got, v)
		}
	}
}
