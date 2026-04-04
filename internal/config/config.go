package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port          string
	DatabaseURL   string
	AllowedOrigin string
	PullLimit     int
	Auth          AuthConfig
	Storage       StorageConfig
}

type AuthConfig struct {
	TokenSecret            string
	TokenTTLMinutes        int
	BootstrapAdminEmail    string
	BootstrapAdminName     string
	BootstrapAdminPassword string
}

type StorageConfig struct {
	Enabled              bool
	Endpoint             string
	AccessKey            string
	SecretKey            string
	Bucket               string
	Region               string
	UseSSL               bool
	PublicBaseURL        string
	PresignExpiryMinutes int
}

func Load() Config {
	loadDotEnv(".env")

	return Config{
		Port:          getEnv("APP_PORT", "8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/pqq?sslmode=disable"),
		AllowedOrigin: getEnv("ALLOWED_ORIGIN", "*"),
		PullLimit:     200,
		Auth: AuthConfig{
			TokenSecret:            getEnv("AUTH_TOKEN_SECRET", ""),
			TokenTTLMinutes:        getIntEnv("AUTH_TOKEN_TTL_MINUTES", 720),
			BootstrapAdminEmail:    getEnv("BOOTSTRAP_ADMIN_EMAIL", ""),
			BootstrapAdminName:     getEnv("BOOTSTRAP_ADMIN_NAME", "System Admin"),
			BootstrapAdminPassword: getEnv("BOOTSTRAP_ADMIN_PASSWORD", ""),
		},
		Storage: StorageConfig{
			Enabled:              getBoolEnv("MINIO_ENABLED", false),
			Endpoint:             getEnv("MINIO_ENDPOINT", ""),
			AccessKey:            getEnv("MINIO_ACCESS_KEY", ""),
			SecretKey:            getEnv("MINIO_SECRET_KEY", ""),
			Bucket:               getEnv("MINIO_BUCKET", ""),
			Region:               getEnv("MINIO_REGION", "us-east-1"),
			UseSSL:               getBoolEnv("MINIO_USE_SSL", true),
			PublicBaseURL:        getEnv("MINIO_PUBLIC_BASE_URL", ""),
			PresignExpiryMinutes: getIntEnv("MINIO_PRESIGN_EXPIRY_MINUTES", 15),
		},
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getBoolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func getIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func loadDotEnv(path string) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		_ = os.Setenv(key, value)
	}
}
