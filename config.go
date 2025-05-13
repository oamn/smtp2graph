// Package main provides configuration loading for smtp2graph from environment variables.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration loaded from environment variables.
//
// Environment variables:
//
//	ENTRA_CLIENT_ID         - Microsoft Entra App registration client ID (required)
//	ENTRA_TENANT_ID         - Microsoft Entra Directory (tenant) ID (required)
//	ENTRA_CLIENT_SECRET     - Microsoft Entra App registration client secret (required)
//	SENDER_EMAIL            - Email address used as sender (required)
//	SENDER_PASSWORD         - Password for the sender email (required)
//	SMTP_SERVER_ADDR        - Address to listen on (default: :1025)
//	SMTP_SERVER_DOMAIN      - SMTP server domain (default: localhost)
//	SMTP_MAX_MESSAGE_BYTES  - Maximum allowed message size in bytes (default: 10485760)
//	SMTP_MAX_RECIPIENTS     - Maximum allowed recipients per message (default: 50)
//	SMTP_WRITE_TIMEOUT      - Write timeout for SMTP connections (default: 10s, e.g. "5s", "1m")
//	SMTP_READ_TIMEOUT       - Read timeout for SMTP connections (default: 10s, e.g. "5s", "1m")
//	SENTRY_DSN              - Sentry DSN for error reporting (optional)

type Config struct {
	SMTPAddr          string        // Address the SMTP server listens on
	SMTPDomain        string        // Domain name for the SMTP server
	MaxMessageBytes   int64         // Maximum allowed message size in bytes
	MaxRecipients     int           // Maximum allowed recipients per message
	WriteTimeout      time.Duration // Write timeout for SMTP connections
	ReadTimeout       time.Duration // Read timeout for SMTP connections
	SenderEmail       string        // Email address used as sender
	SenderPassword    string        // Password for the sender email
	EntraClientID     string        // Microsoft Entra App registration client ID
	EntraTenantID     string        // Microsoft Entra Directory (tenant) ID
	EntraClientSecret string        // Microsoft Entra App registration client secret
	SentryDSN         string        // Sentry DSN for error reporting (optional)
}

// LoadConfig loads configuration from environment variables, applying defaults for SMTP settings.
// Returns an error if required Entra variables are missing.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		SMTPAddr:          getenv("SMTP_SERVER_ADDR", ":1025"),
		SMTPDomain:        getenv("SMTP_SERVER_DOMAIN", "localhost"),
		MaxMessageBytes:   getenvInt64("SMTP_MAX_MESSAGE_BYTES", 10*1024*1024),
		MaxRecipients:     getenvInt("SMTP_MAX_RECIPIENTS", 50),
		WriteTimeout:      getenvDuration("SMTP_WRITE_TIMEOUT", 10*time.Second),
		ReadTimeout:       getenvDuration("SMTP_READ_TIMEOUT", 10*time.Second),
		SenderEmail:       os.Getenv("SENDER_EMAIL"),
		SenderPassword:    os.Getenv("SENDER_PASSWORD"),
		EntraClientID:     os.Getenv("ENTRA_CLIENT_ID"),
		EntraTenantID:     os.Getenv("ENTRA_TENANT_ID"),
		EntraClientSecret: os.Getenv("ENTRA_CLIENT_SECRET"),
		SentryDSN:         os.Getenv("SENTRY_DSN"),
	}

	// Map of required config field names to their values
	required := map[string]string{
		"SENDER_EMAIL":        cfg.SenderEmail,
		"SENDER_PASSWORD":     cfg.SenderPassword,
		"ENTRA_CLIENT_ID":     cfg.EntraClientID,
		"ENTRA_TENANT_ID":     cfg.EntraTenantID,
		"ENTRA_CLIENT_SECRET": cfg.EntraClientSecret,
	}
	var missing []string
	for name, val := range required {
		if val == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

// getenv returns the value of the environment variable or the provided default if unset.
func getenv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

// getenvInt returns the int value of the environment variable or the provided default if unset or invalid.
func getenvInt(key string, def int) int {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	u, err := strconv.ParseUint(val, 10, 0)
	if err != nil || u == 0 {
		return def
	}
	return int(u)
}

// getenvInt64 returns the int64 value of the environment variable or the provided default if unset or invalid.
func getenvInt64(key string, def int64) int64 {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	u, err := strconv.ParseUint(val, 10, 64)
	if err != nil || u == 0 {
		return def
	}
	return int64(u)
}

// getenvDuration returns the time.Duration value of the environment variable or the provided default if unset or invalid.
func getenvDuration(key string, def time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
