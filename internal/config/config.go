package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL        string
	RedisURL           string
	Port               int
	Env                string
	SessionSecret      string
	CORSOrigin         string
	GitHubClientID     string
	GitHubClientSecret string
	StripeSecretKey    string
	StripeWebhookSecret string
}

func (c *Config) GetSessionSecret() string {
	return c.SessionSecret
}

func (c *Config) GetGitHubClientID() string {
	return c.GitHubClientID
}

func (c *Config) GetGitHubClientSecret() string {
	return c.GitHubClientSecret
}

func (c *Config) GetStripeSecretKey() string {
	return c.StripeSecretKey
}

func (c *Config) GetStripeWebhookSecret() string {
	return c.StripeWebhookSecret
}

// Load loads configuration from environment variables.
// In production mode, validates that all required variables are set and secure.
func Load() *Config {
	env := getEnv("API_ENV", "development")
	isDevelopment := env == "development"

	cfg := &Config{
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://saas_user:saas_password@localhost:5432/saas_db?sslmode=disable"),
		RedisURL:           getEnv("REDIS_URL", "redis://localhost:6379"),
		Port:               getEnvAsInt("API_PORT", 8080),
		Env:                env,
		SessionSecret:      getEnv("SESSION_SECRET", "change-me-in-production"),
		CORSOrigin:         getEnv("CORS_ORIGIN", "http://localhost:3000"),
		GitHubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		StripeSecretKey:    getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
	}

	// Validate configuration
	if err := Validate(cfg, isDevelopment); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	return cfg
}

// Validate checks that all required configuration values are set and secure.
// In production mode, fails fast if any required variable is missing or weak.
// In development mode, allows dev defaults.
func Validate(cfg *Config, isDevelopment bool) error {
	var errors []string

	// Always validate critical security settings
	if !isDevelopment {
		// Production: SESSION_SECRET must be set and not be the default
		defaultSessionSecret := "change-me-in-production"
		if cfg.SessionSecret == "" || cfg.SessionSecret == defaultSessionSecret {
			errors = append(errors, "SESSION_SECRET must be set to a secure random value in production")
		}
		if len(cfg.SessionSecret) < 32 {
			errors = append(errors, "SESSION_SECRET must be at least 32 characters long in production")
		}

		// Production: DATABASE_URL must be set and not be the dev default
		defaultDatabaseURL := "postgres://saas_user:saas_password@localhost:5432/saas_db?sslmode=disable"
		if cfg.DatabaseURL == "" || cfg.DatabaseURL == defaultDatabaseURL {
			errors = append(errors, "DATABASE_URL must be set to a production database (cannot use dev default)")
		}

		// Production: REDIS_URL must be set and not be the dev default
		defaultRedisURL := "redis://localhost:6379"
		if cfg.RedisURL == "" || cfg.RedisURL == defaultRedisURL {
			errors = append(errors, "REDIS_URL must be set to a production Redis instance (cannot use dev default)")
		}

		// Production: CORS_ORIGIN should be set to production URL
		defaultCORSOrigin := "http://localhost:3000"
		if cfg.CORSOrigin == "" || cfg.CORSOrigin == defaultCORSOrigin {
			errors = append(errors, "CORS_ORIGIN should be set to your production frontend URL (cannot use localhost)")
		}
	} else {
		// Development: warn about weak secrets but don't fail
		if cfg.SessionSecret == "change-me-in-production" || len(cfg.SessionSecret) < 32 {
			fmt.Fprintf(os.Stderr, "WARNING: Using weak SESSION_SECRET in development. This is unsafe for production.\n")
		}
	}

	// Validate port is in valid range
	if cfg.Port < 1 || cfg.Port > 65535 {
		errors = append(errors, fmt.Sprintf("API_PORT must be between 1 and 65535, got %d", cfg.Port))
	}

	// Validate env is a known value
	validEnvs := []string{"development", "production", "staging", "test"}
	envValid := false
	for _, validEnv := range validEnvs {
		if cfg.Env == validEnv {
			envValid = true
			break
		}
	}
	if !envValid {
		errors = append(errors, fmt.Sprintf("API_ENV must be one of %v, got %s", validEnvs, cfg.Env))
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration errors:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
