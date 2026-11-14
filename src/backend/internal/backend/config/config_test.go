package config

import "testing"

func cleanConfigEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"DB_PATH": "", "DB_MODE": "", "STORAGE_PATH": "", "REST_PORT": "",
		"REST_HOST": "", "WEB_ROOT": "", "CORS_ALLOWED_ORIGINS": "",
		"TRUSTED_PROXIES": "", "AUTH_FILE": "", "AUTH_USERNAME": "",
		"AUTH_PASSWORD": "", "COOKIE_SECURE": "", "ALLOW_INSECURE_COOKIES": "",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func TestLoadConfigRejectsWildcardCORS(t *testing.T) {
	cleanConfigEnvironment(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("wildcard CORS origin must be rejected")
	}
}

func TestLoadConfigRejectsTrustingEveryProxy(t *testing.T) {
	cleanConfigEnvironment(t)
	t.Setenv("TRUSTED_PROXIES", "0.0.0.0/0")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("trusting every proxy must be rejected")
	}
}

func TestLoadConfigRequiresExplicitInsecureCookieOptIn(t *testing.T) {
	cleanConfigEnvironment(t)
	t.Setenv("COOKIE_SECURE", "false")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("insecure cookies must require explicit opt-in")
	}
	t.Setenv("ALLOW_INSECURE_COOKIES", "true")
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("explicit local-development opt-in should be accepted: %v", err)
	}
}
