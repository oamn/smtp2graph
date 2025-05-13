// Package main provides Sentry error reporting integration for smtp2graph.
package main

import (
	"context"
	"log"
	"time"

	"github.com/getsentry/sentry-go"
)

// InitSentry initializes Sentry if a DSN is configured.
// Returns a cleanup function to flush events, or a no-op if Sentry is not enabled.
func InitSentry(cfg *Config) func(context.Context) {
	if cfg.SentryDSN == "" {
		return func(context.Context) {}
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:     cfg.SentryDSN,
		Release: "smtp2graph@" + revision,
	})
	if err != nil {
		log.Fatalf("Sentry initialization failed: %v", err)
	}
	return func(ctx context.Context) {
		sentry.Flush(2 * time.Second)
	}
}

// ReportError sends an error to Sentry if initialized.
func ReportError(ctx context.Context, err error) {
	if err == nil {
		return
	}

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.CaptureException(err)
}
