package main

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigFromDefaults(t *testing.T) {
	cfg, err := loadConfigFrom(configLookup(map[string]string{
		"SENDER_EMAIL":        "sender@example.com",
		"SENDER_PASSWORD":     "password",
		"ENTRA_CLIENT_ID":     "client-id",
		"ENTRA_TENANT_ID":     "tenant-id",
		"ENTRA_CLIENT_SECRET": "client-secret",
	}))
	if err != nil {
		t.Fatalf("loadConfigFrom() error: %v", err)
	}

	if cfg.SMTPAddr != ":1025" {
		t.Errorf("SMTPAddr = %q, want :1025", cfg.SMTPAddr)
	}
	if cfg.SMTPDomain != "localhost" {
		t.Errorf("SMTPDomain = %q, want localhost", cfg.SMTPDomain)
	}
	if cfg.MaxMessageBytes != 10*1024*1024 {
		t.Errorf("MaxMessageBytes = %d, want %d", cfg.MaxMessageBytes, 10*1024*1024)
	}
	if cfg.MaxRecipients != 50 {
		t.Errorf("MaxRecipients = %d, want 50", cfg.MaxRecipients)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout = %s, want 10s", cfg.WriteTimeout)
	}
	if cfg.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %s, want 10s", cfg.ReadTimeout)
	}
}

func TestLoadConfigFromOverrides(t *testing.T) {
	cfg, err := loadConfigFrom(configLookup(map[string]string{
		"SENDER_EMAIL":           "sender@example.com",
		"SENDER_PASSWORD":        "password",
		"ENTRA_CLIENT_ID":        "client-id",
		"ENTRA_TENANT_ID":        "tenant-id",
		"ENTRA_CLIENT_SECRET":    "client-secret",
		"SMTP_SERVER_ADDR":       "127.0.0.1:2525",
		"SMTP_SERVER_DOMAIN":     "mail.example.com",
		"SMTP_MAX_MESSAGE_BYTES": "4096",
		"SMTP_MAX_RECIPIENTS":    "7",
		"SMTP_WRITE_TIMEOUT":     "5s",
		"SMTP_READ_TIMEOUT":      "3s",
		"SENTRY_DSN":             "https://example.invalid/1",
	}))
	if err != nil {
		t.Fatalf("loadConfigFrom() error: %v", err)
	}

	if cfg.SMTPAddr != "127.0.0.1:2525" {
		t.Errorf("SMTPAddr = %q, want 127.0.0.1:2525", cfg.SMTPAddr)
	}
	if cfg.SMTPDomain != "mail.example.com" {
		t.Errorf("SMTPDomain = %q, want mail.example.com", cfg.SMTPDomain)
	}
	if cfg.MaxMessageBytes != 4096 {
		t.Errorf("MaxMessageBytes = %d, want 4096", cfg.MaxMessageBytes)
	}
	if cfg.MaxRecipients != 7 {
		t.Errorf("MaxRecipients = %d, want 7", cfg.MaxRecipients)
	}
	if cfg.WriteTimeout != 5*time.Second {
		t.Errorf("WriteTimeout = %s, want 5s", cfg.WriteTimeout)
	}
	if cfg.ReadTimeout != 3*time.Second {
		t.Errorf("ReadTimeout = %s, want 3s", cfg.ReadTimeout)
	}
	if cfg.SentryDSN != "https://example.invalid/1" {
		t.Errorf("SentryDSN = %q, want configured DSN", cfg.SentryDSN)
	}
}

func TestLoadConfigFromMissingRequired(t *testing.T) {
	_, err := loadConfigFrom(configLookup(nil))
	if err == nil {
		t.Fatal("loadConfigFrom() error = nil, want missing required error")
	}

	for _, name := range []string{
		"ENTRA_CLIENT_ID",
		"ENTRA_CLIENT_SECRET",
		"ENTRA_TENANT_ID",
		"SENDER_EMAIL",
		"SENDER_PASSWORD",
	} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("missing error %q does not include %s", err, name)
		}
	}
}

func TestLoadConfigFromInvalidOptionalValues(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
	}{
		{
			name:    "invalid max message bytes",
			key:     "SMTP_MAX_MESSAGE_BYTES",
			value:   "invalid",
			wantErr: "SMTP_MAX_MESSAGE_BYTES must be a positive integer",
		},
		{
			name:    "zero max recipients",
			key:     "SMTP_MAX_RECIPIENTS",
			value:   "0",
			wantErr: "SMTP_MAX_RECIPIENTS must be a positive integer",
		},
		{
			name:    "invalid write timeout",
			key:     "SMTP_WRITE_TIMEOUT",
			value:   "soon",
			wantErr: "SMTP_WRITE_TIMEOUT must be a positive duration",
		},
		{
			name:    "zero read timeout",
			key:     "SMTP_READ_TIMEOUT",
			value:   "0s",
			wantErr: "SMTP_READ_TIMEOUT must be a positive duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := requiredConfig()
			values[tt.key] = tt.value

			_, err := loadConfigFrom(configLookup(values))
			if err == nil {
				t.Fatal("loadConfigFrom() error = nil, want invalid config error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("loadConfigFrom() error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

func requiredConfig() map[string]string {
	return map[string]string{
		"SENDER_EMAIL":        "sender@example.com",
		"SENDER_PASSWORD":     "password",
		"ENTRA_CLIENT_ID":     "client-id",
		"ENTRA_TENANT_ID":     "tenant-id",
		"ENTRA_CLIENT_SECRET": "client-secret",
	}
}

func configLookup(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
