package config

import (
	"os"
)

type Config struct {
	OIDCProviderURL  string
	OIDCClientID     string
	OIDCClientSecret string
	LogLevel         string
}

func LoadConfig() (*Config, error) {
	return &Config{
		OIDCProviderURL:  getEnv("OIDC_PROVIDER_URL", "https://account.uem.mz/realms/uem"),
		OIDCClientID:     getEnv("OIDC_CLIENT_ID", "radius-client"),
		OIDCClientSecret: getEnv("OIDC_CLIENT_SECRET", ""),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
