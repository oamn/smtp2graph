// Package main provides configuration loading for smtp2graph from environment variables.
package main

import (
	"fmt"
	"net/mail"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// appConfig holds application configuration loaded from environment variables.
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
//	SMTP_MAX_MESSAGE_BYTES  - Maximum allowed message size in bytes (default: 2900000)
//	SMTP_MAX_RECIPIENTS     - Maximum allowed recipients per message (default: 50)
//	SMTP_WRITE_TIMEOUT      - Write timeout for SMTP connections (default: 10s, e.g. "5s", "1m")
//	SMTP_READ_TIMEOUT       - Read timeout for SMTP connections (default: 10s, e.g. "5s", "1m")
//	SENTRY_DSN              - Sentry DSN for error reporting (optional)

type appConfig struct {
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

// maxGraphCompatibleMessageBytes leaves room for base64 expansion and envelope
// headers while keeping the Graph write request below its 4 MB limit.
const maxGraphCompatibleMessageBytes int64 = 2_900_000

// loadConfig loads configuration from environment variables, applying defaults for SMTP settings.
// Returns an error if required variables are missing or optional values are invalid.
func loadConfig() (*appConfig, error) {
	return loadConfigFrom(os.Getenv)
}

// loadConfigFrom loads configuration using lookup and is intended for tests.
func loadConfigFrom(lookup func(string) string) (*appConfig, error) {
	maxMessageBytes, err := getenvInt64(lookup, "SMTP_MAX_MESSAGE_BYTES", maxGraphCompatibleMessageBytes)
	if err != nil {
		return nil, err
	}
	if maxMessageBytes > maxGraphCompatibleMessageBytes {
		return nil, fmt.Errorf("SMTP_MAX_MESSAGE_BYTES must not exceed %d", maxGraphCompatibleMessageBytes)
	}
	maxRecipients, err := getenvInt(lookup, "SMTP_MAX_RECIPIENTS", 50)
	if err != nil {
		return nil, err
	}
	writeTimeout, err := getenvDuration(lookup, "SMTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, err
	}
	readTimeout, err := getenvDuration(lookup, "SMTP_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, err
	}

	cfg := &appConfig{
		SMTPAddr:          getenv(lookup, "SMTP_SERVER_ADDR", ":1025"),
		SMTPDomain:        getenv(lookup, "SMTP_SERVER_DOMAIN", "localhost"),
		MaxMessageBytes:   maxMessageBytes,
		MaxRecipients:     maxRecipients,
		WriteTimeout:      writeTimeout,
		ReadTimeout:       readTimeout,
		SenderEmail:       lookup("SENDER_EMAIL"),
		SenderPassword:    lookup("SENDER_PASSWORD"),
		EntraClientID:     lookup("ENTRA_CLIENT_ID"),
		EntraTenantID:     lookup("ENTRA_TENANT_ID"),
		EntraClientSecret: lookup("ENTRA_CLIENT_SECRET"),
		SentryDSN:         lookup("SENTRY_DSN"),
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
		sort.Strings(missing)
		return nil, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}
	sender, err := mail.ParseAddress(cfg.SenderEmail)
	if err != nil || sender.Name != "" {
		return nil, fmt.Errorf("SENDER_EMAIL must be a valid email address without a display name")
	}
	cfg.SenderEmail = sender.Address
	return cfg, nil
}

// getenv returns the value of the environment variable or the provided default if unset.
func getenv(lookup func(string) string, key, def string) string {
	if val := lookup(key); val != "" {
		return val
	}
	return def
}

// getenvInt returns the int value of the environment variable or the provided default if unset.
func getenvInt(lookup func(string) string, key string, def int) (int, error) {
	val := lookup(key)
	if val == "" {
		return def, nil
	}
	i, err := strconv.ParseInt(val, 10, strconv.IntSize)
	if err != nil || i <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return int(i), nil
}

// getenvInt64 returns the int64 value of the environment variable or the provided default if unset.
func getenvInt64(lookup func(string) string, key string, def int64) (int64, error) {
	val := lookup(key)
	if val == "" {
		return def, nil
	}
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil || i <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return i, nil
}

// getenvDuration returns the time.Duration value of the environment variable or the provided default if unset.
func getenvDuration(lookup func(string) string, key string, def time.Duration) (time.Duration, error) {
	val := lookup(key)
	if val == "" {
		return def, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return d, nil
}
