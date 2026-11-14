package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config represents the application configuration
type Config struct {
	Database DatabaseConfig
	Storage  StorageConfig
	Server   ServerConfig
	Auth     AuthConfig
	Import   ImportConfig
}

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	Path string
	Mode int
}

// StorageConfig represents storage configuration
type StorageConfig struct {
	Path string
}

type ImportConfig struct {
	Roots []string
}

// ServerConfig represents server configuration
type ServerConfig struct {
	RestHost       string
	RestPort       int
	WebRoot        string
	AllowedOrigins []string
	TrustedProxies []string
}

// AuthConfig contains credentials bootstrap and cookie security settings.
type AuthConfig struct {
	File         string
	Username     string
	Password     string
	CookieSecure bool
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{}

	// Database configuration
	config.Database.Path = getEnv("DB_PATH", "./scriptorium.db")

	dbModeStr := getEnv("DB_MODE", "0600")
	dbMode, err := strconv.ParseInt(dbModeStr, 8, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MODE: %s", dbModeStr)
	}
	config.Database.Mode = int(dbMode)

	// Storage configuration
	config.Storage.Path = getEnv("STORAGE_PATH", "./storage")
	for _, root := range filepath.SplitList(os.Getenv("IMPORT_ROOTS")) {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("resolve IMPORT_ROOTS entry %q: %w", root, err)
		}
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("IMPORT_ROOTS entry %q must exist and resolve: %w", root, err)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("IMPORT_ROOTS entry %q must be a directory", root)
		}
		config.Import.Roots = append(config.Import.Roots, filepath.Clean(resolved))
	}

	// Server configuration
	restPortStr := getEnv("REST_PORT", "8080")
	restPort, err := strconv.Atoi(restPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid REST_PORT: %s", restPortStr)
	}
	if restPort < 1 || restPort > 65535 {
		return nil, fmt.Errorf("REST_PORT must be between 1 and 65535")
	}
	config.Server.RestPort = restPort

	config.Server.RestHost = getEnv("REST_HOST", "127.0.0.1")
	config.Server.WebRoot = getEnv("WEB_ROOT", "./web")
	for _, origin := range strings.Split(getEnv("CORS_ALLOWED_ORIGINS", ""), ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || origin == "*" {
			return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS must contain exact origins (scheme and host only): %q", origin)
		}
		config.Server.AllowedOrigins = append(config.Server.AllowedOrigins, origin)
	}
	for _, proxy := range strings.Split(getEnv("TRUSTED_PROXIES", ""), ",") {
		proxy = strings.TrimSpace(proxy)
		if proxy == "" {
			continue
		}
		if net.ParseIP(proxy) == nil {
			_, network, err := net.ParseCIDR(proxy)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES contains an invalid IP or CIDR: %q", proxy)
			}
			ones, bits := network.Mask.Size()
			if ones == 0 || bits == 0 {
				return nil, fmt.Errorf("TRUSTED_PROXIES must not trust all addresses: %q", proxy)
			}
		}
		config.Server.TrustedProxies = append(config.Server.TrustedProxies, proxy)
	}

	config.Auth.File = getEnv("AUTH_FILE", "./scriptorium-auth.json")
	config.Auth.Username = os.Getenv("AUTH_USERNAME")
	config.Auth.Password = os.Getenv("AUTH_PASSWORD")
	cookieSecure, err := strconv.ParseBool(getEnv("COOKIE_SECURE", "true"))
	if err != nil {
		return nil, fmt.Errorf("invalid COOKIE_SECURE: %w", err)
	}
	config.Auth.CookieSecure = cookieSecure
	allowInsecureCookies, err := strconv.ParseBool(getEnv("ALLOW_INSECURE_COOKIES", "false"))
	if err != nil {
		return nil, fmt.Errorf("invalid ALLOW_INSECURE_COOKIES: %w", err)
	}
	if !cookieSecure && !allowInsecureCookies {
		return nil, fmt.Errorf("COOKIE_SECURE=false is only allowed when ALLOW_INSECURE_COOKIES=true (local development only)")
	}

	return config, nil
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
